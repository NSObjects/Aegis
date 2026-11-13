package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

// Universe pools reuse the exact verified per-symbol ScanManager and snapshot
// contract. They are operator-started, sequential, and never invoke an LLM.
const (
	poolBatchSize = 20
	poolMaxSymbols = 4500
	poolSource = "TUSHARE_STOCK_BASIC_L_D_P"
)
type PoolState string
const (
	PoolRunning PoolState = "RUNNING"
	PoolPaused PoolState = "PAUSED"
	PoolFailed PoolState = "FAILED"
	PoolCompleted PoolState = "COMPLETED"
)
var (
	ErrPoolBusy = errors.New("universe pool already running")
	ErrPoolState = errors.New("pool state does not allow operation")
	ErrPoolNotComplete = errors.New("pool is not completely scanned")
)

type PoolRecord struct {
	ID string `json:"pool_id"`
	AsOf string `json:"as_of"`
	StrategyVersion string `json:"strategy_version"`
	UniverseSource string `json:"universe_source"`
	UniverseSHA256 string `json:"universe_sha256"`
	// Frozen PIT membership; never recompute against a later stock_basic list.
	Symbols []string `json:"symbols"`
	State PoolState `json:"state"`
	NextOffset int `json:"next_offset"`
	BatchScanIDs []string `json:"batch_scan_ids"`
	ActiveScanID string `json:"active_scan_id,omitempty"`
	FailureCode string `json:"failure_code,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func poolDigest(asOf, source string, symbols []string) string {
	raw := asOf + "\n" + pullback.StrategyVersion + "\n" + source + "\n" + strings.Join(symbols, "\n")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func validatePool(r PoolRecord) error {
	if !validScanID(r.ID) || r.StrategyVersion != pullback.StrategyVersion || r.UniverseSource != poolSource ||
		r.CreatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) || len(r.Symbols) == 0 || len(r.Symbols) > poolMaxSymbols {
		return errors.New("invalid pool metadata")
	}
	d, err := time.Parse("20060102", r.AsOf)
	if err != nil || d.Format("20060102") != r.AsOf { return errors.New("invalid pool as-of") }
	for i, symbol := range r.Symbols {
		if len(symbol) != 9 || (!strings.HasSuffix(symbol, ".SH") && !strings.HasSuffix(symbol, ".SZ")) ||
			!technical.IsMainBoardSymbol(symbol) || (i > 0 && r.Symbols[i-1] >= symbol) {
			return errors.New("invalid frozen pool symbols")
		}
		for n := 0; n < 6; n++ { if symbol[n] < '0' || symbol[n] > '9' { return errors.New("invalid pool symbol") } }
	}
	if r.UniverseSHA256 != poolDigest(r.AsOf, r.UniverseSource, r.Symbols) {
		return errors.New("pool snapshot digest mismatch")
	}
	if r.NextOffset < 0 || r.NextOffset > len(r.Symbols) ||
		r.NextOffset != len(r.BatchScanIDs)*poolBatchSize && !(r.NextOffset == len(r.Symbols) &&
			len(r.BatchScanIDs) == (len(r.Symbols)+poolBatchSize-1)/poolBatchSize) {
		return errors.New("invalid pool cursor")
	}
	for _, id := range r.BatchScanIDs { if !validScanID(id) { return errors.New("invalid batch ID") } }
	if r.ActiveScanID != "" && !validScanID(r.ActiveScanID) { return errors.New("invalid active batch ID") }
	switch r.State {
	case PoolRunning, PoolPaused, PoolFailed:
		if r.NextOffset >= len(r.Symbols) { return errors.New("unfinished pool already covers universe") }
	case PoolCompleted:
		if r.NextOffset != len(r.Symbols) || r.ActiveScanID != "" || r.FailureCode != "" {
			return errors.New("completed pool has incomplete coverage")
		}
	default: return errors.New("invalid pool state")
	}
	if len(r.FailureCode) > 64 { return errors.New("pool failure code too long") }
	return nil
}

// FilePoolStore uses atomic private files. A pool references immutable
// per-batch ScanRecords, whose own snapshots are digest-checked on comparison.
type FilePoolStore struct { dir string; mu sync.Mutex }
func NewFilePoolStore(dir string) (*FilePoolStore, error) {
	if strings.TrimSpace(dir) == "" { return nil, errors.New("pool directory required") }
	if err := os.MkdirAll(dir, 0700); err != nil { return nil, err }
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 { return nil, errors.New("pool directory must be private") }
	return &FilePoolStore{dir: dir}, nil
}
func (s *FilePoolStore) Save(r PoolRecord) error {
	if s == nil { return errors.New("nil pool store") }
	if err := validatePool(r); err != nil { return err }
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dir, r.ID+".json")
	if raw, err := os.ReadFile(path); err == nil {
		var old PoolRecord
		if json.Unmarshal(raw, &old) != nil || validatePool(old) != nil ||
			old.ID != r.ID || old.AsOf != r.AsOf || old.UniverseSHA256 != r.UniverseSHA256 ||
			!old.CreatedAt.Equal(r.CreatedAt) || old.NextOffset > r.NextOffset ||
			len(old.BatchScanIDs) > len(r.BatchScanIDs) {
			return errors.New("pool immutable inputs or cursor changed")
		}
		for i := range old.BatchScanIDs {
			if old.BatchScanIDs[i] != r.BatchScanIDs[i] { return errors.New("pool committed batch changed") }
		}
		if old.State == PoolCompleted && r.State != PoolCompleted { return errors.New("terminal pool changed") }
	} else if !os.IsNotExist(err) { return err }
	raw, err := json.Marshal(r)
	if err != nil { return err }
	tmp, err := os.CreateTemp(s.dir, ".pool-*.tmp")
	if err != nil { return err }
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0600); err == nil { _, err = tmp.Write(raw) }
	if err == nil { err = tmp.Sync() }
	closeErr := tmp.Close()
	if err != nil { return err }
	if closeErr != nil { return closeErr }
	return os.Rename(tmp.Name(), path)
}
func (s *FilePoolStore) Get(id string) (PoolRecord, error) {
	if s == nil || !validScanID(id) { return PoolRecord{}, errors.New("invalid pool lookup") }
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if err != nil { return PoolRecord{}, err }
	if len(raw) > 150000 { return PoolRecord{}, errors.New("pool record too large") }
	var r PoolRecord
	if err := json.Unmarshal(raw, &r); err != nil { return PoolRecord{}, err }
	if err := validatePool(r); err != nil { return PoolRecord{}, err }
	if r.ID != id { return PoolRecord{}, errors.New("pool filename identity mismatch") }
	return r, nil
}
func (s *FilePoolStore) List(limit int) ([]PoolRecord, error) {
	if s == nil || limit < 1 || limit > 100 { return nil, errors.New("invalid pool list limit") }
	entries, err := os.ReadDir(s.dir)
	if err != nil { return nil, err }
	records := make([]PoolRecord, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") { continue }
		id := strings.TrimSuffix(e.Name(), ".json")
		r, err := s.Get(id)
		if err != nil { return nil, err }
		records = append(records, r)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) { return records[i].ID > records[j].ID }
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	if len(records) > limit { records = records[:limit] }
	return records, nil
}
func (s *FilePoolStore) ReconcileInterrupted() error {
	records, err := s.List(100)
	if err != nil { return err }
	for _, rec := range records {
		if rec.State == PoolRunning {
			rec.State = PoolPaused
			rec.FailureCode = "PROCESS_INTERRUPTED"
			rec.UpdatedAt = time.Now().UTC()
			if err := s.Save(rec); err != nil { return err }
		}
	}
	return nil
}

type PoolUniverseProvider interface {
	ListStockUniverse(context.Context, string) (technical.StockUniverse, error)
}
type PoolManager struct {
	store *FilePoolStore
	scans *ScanManager
	universe PoolUniverseProvider
	ctx context.Context
	mu sync.Mutex
	runningID string
	pauseRequested bool
}
func NewPoolManager(ctx context.Context, store *FilePoolStore, scans *ScanManager, universe PoolUniverseProvider) (*PoolManager, error) {
	if ctx == nil || store == nil || scans == nil || universe == nil { return nil, errors.New("pool dependencies required") }
	if err := store.ReconcileInterrupted(); err != nil { return nil, err }
	return &PoolManager{store: store, scans: scans, universe: universe, ctx: ctx}, nil
}
func (m *PoolManager) startJob(rec PoolRecord) {
	m.runningID = rec.ID
	m.pauseRequested = false
	go m.run(rec.ID)
}
func (m *PoolManager) Start(ctx context.Context, asOf string) (PoolRecord, error) {
	d, err := time.Parse("20060102", asOf)
	if err != nil || d.Format("20060102") != asOf ||
		asOf > time.Now().In(time.FixedZone("CST",8*3600)).Format("20060102") {
		return PoolRecord{}, errors.New("invalid or future market cutoff")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runningID != "" { return PoolRecord{}, ErrPoolBusy }
	// ListStockUniverse performs only 3 listing-status requests, before any
	// per-security scans. Freeze the complete PIT membership once.
	snapshot, err := m.universe.ListStockUniverse(ctx, asOf)
	if err != nil { return PoolRecord{}, fmt.Errorf("stock universe unavailable: %w", err) }
	if snapshot.AsOf != asOf || snapshot.Source != poolSource || len(snapshot.Symbols) > poolMaxSymbols {
		return PoolRecord{}, errors.New("unsupported PIT universe")
	}
	id, err := NewScanID()
	if err != nil { return PoolRecord{}, err }
	now := time.Now().UTC()
	rec := PoolRecord{ID:id, AsOf:asOf, StrategyVersion:pullback.StrategyVersion,
		UniverseSource:poolSource, UniverseSHA256:poolDigest(asOf,poolSource,snapshot.Symbols),
		Symbols:append([]string(nil),snapshot.Symbols...), State:PoolRunning,
		BatchScanIDs:[]string{}, CreatedAt:now, UpdatedAt:now}
	if err := m.store.Save(rec); err != nil { return PoolRecord{}, err }
	m.startJob(rec)
	return rec, nil
}
func (m *PoolManager) Resume(id string) (PoolRecord,error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runningID != "" { return PoolRecord{}, ErrPoolBusy }
	rec, err := m.store.Get(id)
	if err != nil { return PoolRecord{}, err }
	if rec.State != PoolPaused && rec.State != PoolFailed { return PoolRecord{}, ErrPoolState }
	rec.State = PoolRunning
	rec.FailureCode = ""
	rec.UpdatedAt = time.Now().UTC()
	if err := m.store.Save(rec); err != nil { return PoolRecord{}, err }
	m.startJob(rec)
	return rec,nil
}
func (m *PoolManager) Pause(id string) (PoolRecord,error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec,err := m.store.Get(id)
	if err != nil { return PoolRecord{},err }
	if m.runningID != id || rec.State != PoolRunning { return PoolRecord{},ErrPoolState }
	// Stop at the next durable batch boundary. Never lose an in-flight batch.
	m.pauseRequested = true
	return rec,nil
}
func (m *PoolManager) Get(id string) (PoolRecord,error) { return m.store.Get(id) }
func (m *PoolManager) List(limit int) ([]PoolRecord,error) { return m.store.List(limit) }

func (m *PoolManager) stop(rec PoolRecord, state PoolState, code string) {
	rec.State = state
	rec.FailureCode = code
	rec.UpdatedAt = time.Now().UTC()
	_ = m.store.Save(rec) // On IO errors, next startup reconciles RUNNING -> PAUSED.
}
func (m *PoolManager) run(id string) {
	defer func() {
		m.mu.Lock()
		if m.runningID == id { m.runningID = ""; m.pauseRequested = false }
		m.mu.Unlock()
	}()
	rec,err := m.store.Get(id)
	if err != nil { return }
	for rec.NextOffset < len(rec.Symbols) {
		select {
		case <-m.ctx.Done(): return // restart reconciliation prevents silent relaunch
		default:
		}
		// An interrupted run can have a durable active scan whose terminal
		// completion was never copied to its parent cursor.
		if rec.ActiveScanID != "" {
			batch,err := m.scans.Get(rec.ActiveScanID)
			if err != nil { m.stop(rec, PoolFailed, "SCAN_READ_FAILED"); return }
			if batch.State == ScanPending || batch.State == ScanRunning {
				if !m.awaitBatch(&rec, batch.ID) { return }
				batch,err = m.scans.Get(batch.ID)
				if err != nil { m.stop(rec,PoolFailed,"SCAN_READ_FAILED"); return }
			}
			if batch.State == ScanCompleted {
				end := rec.NextOffset + poolBatchSize
				if end > len(rec.Symbols) { end = len(rec.Symbols) }
				if batch.AsOf != rec.AsOf || batch.UniverseSource != rec.UniverseSource ||
					len(batch.Symbols) != end-rec.NextOffset {
					m.stop(rec,PoolFailed,"BATCH_IDENTITY_MISMATCH"); return
				}
				for i,s := range batch.Symbols {
					if s != rec.Symbols[rec.NextOffset+i] { m.stop(rec,PoolFailed,"BATCH_IDENTITY_MISMATCH"); return }
				}
				rec.BatchScanIDs = append(rec.BatchScanIDs,batch.ID)
				rec.NextOffset = end
				rec.ActiveScanID = ""
				rec.UpdatedAt = time.Now().UTC()
				if rec.NextOffset == len(rec.Symbols) { rec.State = PoolCompleted }
				if err := m.store.Save(rec); err != nil { return }
				continue
			}
			// A failed batch never advances coverage. Resuming reissues that
			// same slice; the failed child scan remains available for audit.
			rec.ActiveScanID = ""
			rec.UpdatedAt = time.Now().UTC()
			if err := m.store.Save(rec); err != nil { return }
			m.stop(rec,PoolFailed,"BATCH_FAILED")
			return
		}
		m.mu.Lock()
		pause := m.pauseRequested
		m.mu.Unlock()
		if pause { m.stop(rec,PoolPaused,"OPERATOR_PAUSED"); return }
		end := rec.NextOffset + poolBatchSize
		if end > len(rec.Symbols) { end = len(rec.Symbols) }
		batch,err := m.scans.Submit(m.ctx,rec.AsOf,rec.UniverseSource,rec.Symbols[rec.NextOffset:end])
		if err != nil {
			m.stop(rec,PoolPaused,"SCAN_BUSY_OR_UNAVAILABLE")
			return
		}
		rec.ActiveScanID = batch.ID
		rec.UpdatedAt = time.Now().UTC()
		if err := m.store.Save(rec); err != nil { return }
		// The scan owns a 3-minute per-batch deadline, so a failed provider
		// stops the pool rather than immediately retrying unbounded requests.
		if !m.awaitBatch(&rec,batch.ID) { return }
	}
	if rec.State != PoolCompleted { m.stop(rec,PoolCompleted,"") }
}
func (m *PoolManager) awaitBatch(rec *PoolRecord, id string) bool {
	ticker := time.NewTicker(300*time.Millisecond)
	defer ticker.Stop()
	for {
		batch,err := m.scans.Get(id)
		if err != nil { m.stop(*rec,PoolFailed,"SCAN_READ_FAILED"); return false }
		if batch.State == ScanCompleted || batch.State == ScanFailed { return true }
		select {
		case <-m.ctx.Done(): return false
		case <-ticker.C:
		}
	}
}

type PoolRanked struct {
	Rank int `json:"rank"`
	Symbol string `json:"symbol"`
	Metrics pullback.Metrics `json:"metrics"`
	ScanID string `json:"scan_id"`
	Source ScanComparisonSource `json:"source"`
}
type PoolComparison struct {
	PoolID string `json:"pool_id"`
	AsOf string `json:"as_of"`
	StrategyVersion string `json:"strategy_version"`
	UniverseSHA256 string `json:"universe_sha256"`
	UniverseCount int `json:"universe_count"`
	SelectedCount int `json:"selected_count"`
	IncomparableCount int `json:"incomparable_count"`
	ReasonCounts map[string]int `json:"reason_counts"`
	RankingRule string `json:"ranking_rule"`
	Offset int `json:"offset"`
	Limit int `json:"limit"`
	Ranked []PoolRanked `json:"ranked"`
}
func (m *PoolManager) Comparison(id string,offset,limit int) (PoolComparison,error) {
	if offset < 0 || limit < 1 || limit > 100 { return PoolComparison{},errors.New("invalid pool comparison window") }
	rec,err := m.store.Get(id)
	if err != nil { return PoolComparison{},err }
	if rec.State != PoolCompleted { return PoolComparison{},ErrPoolNotComplete }
	all := make([]pullback.Candidate,0)
	meta := map[string]PoolRanked{}
	reasons := map[string]int{}
	totalIncomparable := 0
	for n,scanID := range rec.BatchScanIDs {
		batch,err := m.scans.Get(scanID)
		if err != nil { return PoolComparison{},err }
		start := n*poolBatchSize
		end := start+poolBatchSize
		if end > len(rec.Symbols) { end = len(rec.Symbols) }
		if batch.State != ScanCompleted || batch.AsOf != rec.AsOf ||
			batch.UniverseSource != rec.UniverseSource || len(batch.Symbols) != end-start {
			return PoolComparison{},errors.New("pool child scan identity mismatch")
		}
		for i,s := range batch.Symbols { if rec.Symbols[start+i] != s { return PoolComparison{},errors.New("pool batch membership mismatch") } }
		view,err := m.scans.Comparison(scanID)
		if err != nil { return PoolComparison{},err }
		for _, r := range view.Ranked {
			if _,exists := meta[r.Symbol]; exists { return PoolComparison{},errors.New("duplicate ranked candidate") }
			all = append(all,pullback.Candidate{Symbol:r.Symbol,Metrics:r.Metrics})
			meta[r.Symbol] = PoolRanked{Symbol:r.Symbol,Metrics:r.Metrics,ScanID:scanID,Source:r.Source}
		}
		for _,r := range view.Incomparable { totalIncomparable++; reasons[r.Reason]++ }
	}
	if len(all)+totalIncomparable != len(rec.Symbols) { return PoolComparison{},errors.New("pool coverage mismatch") }
	sorted := pullback.RankCandidates(all)
	out := PoolComparison{PoolID:id,AsOf:rec.AsOf,StrategyVersion:rec.StrategyVersion,
		UniverseSHA256:rec.UniverseSHA256,UniverseCount:len(rec.Symbols),
		SelectedCount:len(all),IncomparableCount:totalIncomparable,
		ReasonCounts:reasons,RankingRule:pullback.RankingRule,
		Offset:offset,Limit:limit,Ranked:[]PoolRanked{}}
	if offset >= len(sorted) { return out,nil }
	end := offset+limit
	if end > len(sorted) { end = len(sorted) }
	for i:=offset;i<end;i++ {
		row:=meta[sorted[i].Symbol]
		row.Rank=i+1
		out.Ranked=append(out.Ranked,row)
	}
	return out,nil
}
