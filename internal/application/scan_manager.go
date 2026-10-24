package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

// ScanState is the durable lifecycle of one deterministic screening scan.
// None of these states represent an order, trade execution, or approval.
type ScanState string

const (
	ScanPending   ScanState = "PENDING"
	ScanRunning   ScanState = "RUNNING"
	ScanCompleted ScanState = "COMPLETED"
	ScanFailed    ScanState = "FAILED"
	ScanCancelled ScanState = "CANCELLED"
)

const (
	ScanFailureCredential     = "CREDENTIAL_UNAVAILABLE"
	ScanFailureData           = "DATA_UNAVAILABLE"
	ScanFailureNotTradingDay  = "NOT_A_TRADING_DAY"
	ScanFailureUnknownSymbol  = "UNKNOWN_SYMBOL"
	ScanFailurePersistence    = "PERSISTENCE_FAILED"
	ScanFailureTimeout        = "SCAN_TIMEOUT"
	ScanFailureInterrupted    = "PROCESS_INTERRUPTED"
)

// The screening lookback is fixed per scan request so that replaying the same
// as-of date and universe reproduces the same evaluation window.
const (
	ScanLookbackTradingDays = 90
	maxExplicitScanSymbols  = 500
)

// ScanRequest is the submitted scan input. Symbols is nil or empty for the
// full listed SH/SZ market, or an explicit subset validated against listings.
type ScanRequest struct {
	IdempotencyKey string
	AsOf           string
	Symbols        []string
}

// ScanRequestSnapshot is the immutable, non-sensitive projection persisted
// with every scan record. It fully determines a replay: same as-of date,
// same strategy version, same universe policy, same lookback.
type ScanRequestSnapshot struct {
	SchemaVersion        int      `json:"schema_version"`
	AsOf                 string   `json:"as_of"`
	StrategyVersion      string   `json:"strategy_version"`
	UniversePolicy       string   `json:"universe_policy"`
	Symbols              []string `json:"symbols,omitempty"`
	LookbackTradingDays  int      `json:"lookback_trading_days"`
}

// ScanResultItem is one symbol's outcome. SnapshotSHA256 anchors the exact
// observed input bars so any verdict can be re-verified offline.
type ScanResultItem struct {
	Symbol        string `json:"symbol"`
	Verdict       string `json:"verdict"`
	Reason        string `json:"reason"`
	BarsCount     int    `json:"bars_count"`
	FirstBarDate  string `json:"first_bar_date,omitempty"`
	LastBarDate   string `json:"last_bar_date,omitempty"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
}

// ScanSummary counts verdicts across all evaluated symbols.
type ScanSummary struct {
	Total       int `json:"total"`
	Selected    int `json:"selected"`
	Rejected    int `json:"rejected"`
	Insufficient int `json:"insufficient"`
	Errored     int `json:"errored"`
}

// ScanProgress exposes running advancement without streaming events.
type ScanProgress struct {
	SymbolsTotal     int `json:"symbols_total"`
	SymbolsEvaluated int `json:"symbols_evaluated"`
	DataDaysTotal    int `json:"data_days_total"`
	DataDaysFetched  int `json:"data_days_fetched"`
}

// ScanEvent is a persisted, replayable, non-sensitive execution event.
type ScanEvent struct {
	Seq    uint64    `json:"seq"`
	Stage  string    `json:"stage"`
	Status string    `json:"status"`
	At     time.Time `json:"at"`
}

// ScanRecord stores scan metadata and per-symbol outcomes. Bars snapshots
// live in separate per-symbol files referenced by SnapshotSHA256.
type ScanRecord struct {
	IdempotencyDigest  string              `json:"idempotency_digest,omitempty"`
	RequestFingerprint string              `json:"request_fingerprint,omitempty"`
	ID                 string              `json:"scan_id"`
	State              ScanState           `json:"state"`
	RequestSnapshot    *ScanRequestSnapshot `json:"request_snapshot,omitempty"`
	Progress           ScanProgress        `json:"progress"`
	// Results may be partial while RUNNING and after FAILED/CANCELLED.
	Results    []ScanResultItem `json:"results,omitempty"`
	Summary    *ScanSummary     `json:"summary,omitempty"`
	FailureCode string          `json:"failure_code,omitempty"`
	Events     []ScanEvent      `json:"events,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
	StartedAt  *time.Time       `json:"started_at,omitempty"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
}

// ScanStore is a single-writer durable scan repository plus the per-symbol
// input-snapshot side files. Implementations must return detached snapshots.
type ScanStore interface {
	Save(context.Context, ScanRecord) error
	Get(context.Context, string) (ScanRecord, error)
	List(context.Context) ([]ScanRecord, error)
	SaveSnapshot(ctx context.Context, scanID, symbol string, bars []technical.HistoricalBar) (string, error)
	GetSnapshot(ctx context.Context, scanID, symbol, expectedSHA256 string) ([]technical.HistoricalBar, error)
}

var (
	ErrScanNotFound          = errors.New("strategy scan not found")
	ErrScanNotActive         = errors.New("strategy scan is not active")
	ErrScanCapacity          = errors.New("strategy scan queue is at capacity")
	ErrScanManagerClosed     = errors.New("strategy scan manager closed")
	ErrScanIdempotencyConflict = errors.New("idempotency key already used for a different scan request")
	ErrScanSymbolNotFound    = errors.New("scan result for symbol not found")
	ErrScanNotTerminal       = errors.New("scan snapshots are unavailable until the scan reaches a terminal state")
	ErrInvalidScanRequest    = errors.New("invalid scan request")
)

// ScanSourceFactory resolves the batch market data source at execution time,
// mirroring how live research loads fresh credentials per run.
type ScanSourceFactory func(ctx context.Context) (technical.BatchMarketSource, error)

type scanKeyIndexEntry struct {
	ScanID      string
	Fingerprint string
}

// ScanManager schedules bounded, asynchronous deterministic screening scans.
// Scans never invoke an LLM and never place orders.
type ScanManager struct {
	factory ScanSourceFactory
	store   ScanStore
	mu      sync.Mutex
	active  map[string]context.CancelFunc
	idempotency map[string]scanKeyIndexEntry
	closed  bool
	closeDone chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	slots    chan struct{}
	maxActive int
	scanTimeout time.Duration
	wg       sync.WaitGroup
	now      func() time.Time
}

// NewScanManager reconciles process-interrupted PENDING/RUNNING scans to
// FAILED before accepting new work. maxActive bounds running plus queued
// scans; only one scan executes at a time.
func NewScanManager(factory ScanSourceFactory, store ScanStore, maxActive int) (*ScanManager, error) {
	if factory == nil || store == nil {
		return nil, errors.New("scan manager requires source factory and store")
	}
	if maxActive < 1 {
		return nil, errors.New("invalid scan concurrency limit")
	}
	existing, err := store.List(context.Background())
	if err != nil {
		return nil, fmt.Errorf("load strategy scans: %w", err)
	}
	index := make(map[string]scanKeyIndexEntry)
	for _, record := range existing {
		if record.IdempotencyDigest != "" {
			if previous, exists := index[record.IdempotencyDigest]; exists {
				return nil, fmt.Errorf("duplicate persisted idempotency key for scans %s and %s", previous.ScanID, record.ID)
			}
			index[record.IdempotencyDigest] = scanKeyIndexEntry{ScanID: record.ID, Fingerprint: record.RequestFingerprint}
		}
	}
	now := time.Now().UTC()
	for _, record := range existing {
		if record.State != ScanPending && record.State != ScanRunning {
			continue
		}
		record.State = ScanFailed
		record.FinishedAt = &now
		record.FailureCode = ScanFailureInterrupted
		record.Events = append(record.Events, ScanEvent{Seq: uint64(len(record.Events) + 1), Stage: "Scan", Status: string(ScanFailed), At: now})
		if err := store.Save(context.Background(), record); err != nil {
			return nil, fmt.Errorf("reconcile interrupted scan %s: %w", record.ID, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ScanManager{factory: factory, store: store, active: make(map[string]context.CancelFunc), idempotency: index, ctx: ctx, cancel: cancel, slots: make(chan struct{}, 1), maxActive: maxActive, scanTimeout: 20 * time.Minute, now: func() time.Time { return time.Now().UTC() }}, nil
}

// ValidateScanAsOf enforces the canonical YYYYMMDD trading-date format.
func ValidateScanAsOf(asOf string) error {
	if len(asOf) != 8 {
		return errors.New("as_of must be YYYYMMDD")
	}
	parsed, err := time.Parse("20060102", asOf)
	if err != nil || parsed.Format("20060102") != asOf {
		return errors.New("as_of must be a valid calendar date")
	}
	return nil
}

// ValidateScanSymbol accepts only the SH/SZ daily-bar symbol shape; the
// strategy universe prefixes are evaluated and recorded per symbol.
func ValidateScanSymbol(symbol string) error {
	if len(symbol) != 9 {
		return errors.New("symbol must be 6 digits with .SH or .SZ suffix")
	}
	if !strings.HasSuffix(symbol, ".SH") && !strings.HasSuffix(symbol, ".SZ") {
		return errors.New("symbol must have .SH or .SZ suffix")
	}
	for i := 0; i < 6; i++ {
		if symbol[i] < '0' || symbol[i] > '9' {
			return errors.New("symbol must be 6 digits with .SH or .SZ suffix")
		}
	}
	return nil
}

func scanRequestFingerprint(asOf string, symbols []string) (string, error) {
	canonical := struct {
		AsOf        string   `json:"as_of"`
		Symbols     []string `json:"symbols"`
		Lookback    int      `json:"lookback_trading_days"`
		Strategy    string   `json:"strategy_version"`
	}{AsOf: asOf, Symbols: symbols, Lookback: ScanLookbackTradingDays, Strategy: pullback.StrategyVersion}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("fingerprint scan request: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func snapshotScanRequest(request ScanRequest) *ScanRequestSnapshot {
	snapshot := &ScanRequestSnapshot{
		SchemaVersion:       1,
		AsOf:                request.AsOf,
		StrategyVersion:     pullback.StrategyVersion,
		UniversePolicy:      "LISTED_SH_SZ",
		LookbackTradingDays: ScanLookbackTradingDays,
	}
	if len(request.Symbols) > 0 {
		snapshot.Symbols = append([]string(nil), request.Symbols...)
	}
	return snapshot
}

func (m *ScanManager) Submit(ctx context.Context, request ScanRequest) (ScanRecord, error) {
	if m == nil {
		return ScanRecord{}, errors.New("scan manager is nil")
	}
	if ctx == nil {
		return ScanRecord{}, errors.New("submit context is nil")
	}
	if err := ctx.Err(); err != nil {
		return ScanRecord{}, err
	}
	request.AsOf = strings.TrimSpace(request.AsOf)
	if err := ValidateScanAsOf(request.AsOf); err != nil {
		return ScanRecord{}, fmt.Errorf("%w: %w", ErrInvalidScanRequest, err)
	}
	seen := make(map[string]struct{}, len(request.Symbols))
	symbols := make([]string, 0, len(request.Symbols))
	for _, symbol := range request.Symbols {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if err := ValidateScanSymbol(symbol); err != nil {
			return ScanRecord{}, fmt.Errorf("%w: %w", ErrInvalidScanRequest, err)
		}
		if _, duplicate := seen[symbol]; duplicate {
			return ScanRecord{}, fmt.Errorf("%w: duplicate symbol %s", ErrInvalidScanRequest, symbol)
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	if len(symbols) > maxExplicitScanSymbols {
		return ScanRecord{}, fmt.Errorf("%w: explicit symbols exceed %d", ErrInvalidScanRequest, maxExplicitScanSymbols)
	}
	sort.Strings(symbols)
	request.Symbols = symbols
	key := request.IdempotencyKey
	if err := ValidateIdempotencyKey(key); err != nil {
		return ScanRecord{}, err
	}
	var keyDigest, requestFingerprint string
	if key != "" {
		keyHash := sha256.Sum256([]byte(key))
		keyDigest = hex.EncodeToString(keyHash[:])
		fingerprint, err := scanRequestFingerprint(request.AsOf, request.Symbols)
		if err != nil {
			return ScanRecord{}, err
		}
		requestFingerprint = fingerprint
	}
	id, err := secureRunID()
	if err != nil {
		return ScanRecord{}, err
	}
	now := m.now()
	record := ScanRecord{
		IdempotencyDigest: keyDigest, RequestFingerprint: requestFingerprint,
		ID: id, State: ScanPending, RequestSnapshot: snapshotScanRequest(request),
		CreatedAt: now,
		Events: []ScanEvent{{Seq: 1, Stage: "Scan", Status: string(ScanPending), At: now}},
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ScanRecord{}, ErrScanManagerClosed
	}
	if keyDigest != "" {
		if previous, exists := m.idempotency[keyDigest]; exists {
			if previous.Fingerprint != requestFingerprint {
				return ScanRecord{}, ErrScanIdempotencyConflict
			}
			return m.store.Get(ctx, previous.ScanID)
		}
	}
	if len(m.active) >= m.maxActive {
		return ScanRecord{}, ErrScanCapacity
	}
	if err := ctx.Err(); err != nil {
		return ScanRecord{}, err
	}
	if err := m.store.Save(ctx, record); err != nil {
		return ScanRecord{}, fmt.Errorf("persist new strategy scan: %w", err)
	}
	runCtx, cancel := context.WithTimeout(m.ctx, m.scanTimeout)
	if keyDigest != "" {
		m.idempotency[keyDigest] = scanKeyIndexEntry{ScanID: id, Fingerprint: requestFingerprint}
	}
	m.active[id] = cancel
	m.wg.Add(1)
	go m.execute(runCtx, record, cancel)
	returned := record
	returned.RequestSnapshot = detachScanSnapshot(record.RequestSnapshot)
	returned.Events = append([]ScanEvent(nil), record.Events...)
	return returned, nil
}

func detachScanSnapshot(snapshot *ScanRequestSnapshot) *ScanRequestSnapshot {
	if snapshot == nil {
		return nil
	}
	copy := *snapshot
	copy.Symbols = append([]string(nil), snapshot.Symbols...)
	return &copy
}

func (m *ScanManager) execute(ctx context.Context, record ScanRecord, cancel context.CancelFunc) {
	defer cancel()
	defer m.wg.Done()
	defer func() { m.mu.Lock(); delete(m.active, record.ID); m.mu.Unlock() }()

	var mu sync.Mutex
	// persist and appendEvent expect the caller to hold mu; terminal takes it.
	persist := func() error {
		return m.store.Save(context.Background(), record)
	}
	appendEvent := func(stage, status string, at time.Time) error {
		record.Events = append(record.Events, ScanEvent{Seq: uint64(len(record.Events) + 1), Stage: stage, Status: status, At: at})
		return m.store.Save(context.Background(), record)
	}
	terminal := func(state ScanState, code string) {
		mu.Lock()
		defer mu.Unlock()
		if record.State != ScanPending && record.State != ScanRunning {
			return
		}
		done := m.now()
		record.State = state
		record.FinishedAt = &done
		record.FailureCode = code
		if state == ScanCompleted {
			record.Summary = summarizeScan(record.Results)
		}
		_ = appendEvent("Scan", string(state), done)
	}
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			terminal(ScanFailed, ScanFailureTimeout)
		} else {
			terminal(ScanCancelled, "")
		}
		return
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			terminal(ScanFailed, ScanFailureTimeout)
		} else {
			terminal(ScanCancelled, "")
		}
		return
	}
	mu.Lock()
	started := m.now()
	record.StartedAt = &started
	record.State = ScanRunning
	err := appendEvent("Scan", string(ScanRunning), started)
	mu.Unlock()
	if err != nil {
		terminal(ScanFailed, ScanFailurePersistence)
		return
	}
	// A stalled HTTP fetch may ignore ctx; record deadline/cancellation
	// independently while the slot is retained until the call returns.
	watchdogDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				terminal(ScanFailed, ScanFailureTimeout)
			} else {
				terminal(ScanCancelled, "")
			}
		case <-watchdogDone:
		}
	}()
	defer close(watchdogDone)
	source, err := m.factory(ctx)
	if err != nil {
		terminal(ScanFailed, ScanFailureCredential)
		return
	}
	asOf := record.RequestSnapshot.AsOf
	lookback := record.RequestSnapshot.LookbackTradingDays

	// Calendar: the as-of date must itself be an open trading day, otherwise
	// every symbol would fail with missing bars for a predictable reason.
	days, err := source.TradingDays(ctx, asOf, lookback)
	if err != nil {
		terminal(ScanFailed, ScanFailureData)
		return
	}
	if len(days) != lookback || days[len(days)-1] != asOf {
		if len(days) > 0 && days[len(days)-1] != asOf {
			terminal(ScanFailed, ScanFailureNotTradingDay)
		} else {
			terminal(ScanFailed, ScanFailureData)
		}
		return
	}
	listed, err := source.ListedSymbols(ctx)
	if err != nil {
		terminal(ScanFailed, ScanFailureData)
		return
	}
	listedSet := make(map[string]struct{}, len(listed))
	for _, symbol := range listed {
		listedSet[symbol] = struct{}{}
	}
	var universe []string
	if explicit := record.RequestSnapshot.Symbols; len(explicit) > 0 {
		for _, symbol := range explicit {
			if _, ok := listedSet[symbol]; !ok {
				terminal(ScanFailed, ScanFailureUnknownSymbol)
				return
			}
		}
		universe = append([]string(nil), explicit...)
	} else {
		for _, symbol := range listed {
			if strings.HasSuffix(symbol, ".SH") || strings.HasSuffix(symbol, ".SZ") {
				universe = append(universe, symbol)
			}
		}
		sort.Strings(universe)
	}
	mu.Lock()
	record.Progress = ScanProgress{SymbolsTotal: len(universe), DataDaysTotal: len(days)}
	err = appendEvent("Universe", "DONE", m.now())
	mu.Unlock()
	if err != nil {
		terminal(ScanFailed, ScanFailurePersistence)
		return
	}

	// Market data: fetch each trading day once, market-wide. A symbol absent
	// on a day simply keeps no bar for it; absence is never inferred.
	barsBySymbol := make(map[string][]technical.HistoricalBar)
	for _, day := range days {
		if ctx.Err() != nil {
			return // watchdog records the terminal state
		}
		batch, err := source.DailyBars(ctx, day)
		if err != nil {
			if ctx.Err() != nil {
				return // watchdog records cancellation or timeout
			}
			terminal(ScanFailed, ScanFailureData)
			return
		}
		seen := make(map[string]struct{}, len(batch))
		for i := range batch {
			symbol := batch[i].Symbol
			if _, duplicate := seen[symbol]; duplicate {
				terminal(ScanFailed, ScanFailureData)
				return
			}
			seen[symbol] = struct{}{}
			barsBySymbol[symbol] = append(barsBySymbol[symbol], technical.HistoricalBar{
				TradeDate: batch[i].TradeDate,
				Open:      batch[i].Open,
				High:      batch[i].High,
				Low:       batch[i].Low,
				Close:     batch[i].Close,
				Volume:    batch[i].Volume,
				Amount:    batch[i].Amount,
			})
		}
		mu.Lock()
		record.Progress.DataDaysFetched++
		// Persist progress in coarse batches; every day would fsync the whole
		// record ninety times per scan without changing observable semantics.
		if record.Progress.DataDaysFetched%15 == 0 || record.Progress.DataDaysFetched == len(days) {
			err = persist()
		}
		mu.Unlock()
		if err != nil {
			terminal(ScanFailed, ScanFailurePersistence)
			return
		}
	}
	mu.Lock()
	err = appendEvent("MarketData", "DONE", m.now())
	mu.Unlock()
	if err != nil {
		terminal(ScanFailed, ScanFailurePersistence)
		return
	}

	// Evaluation: deterministic per symbol; missing data is an explicit
	// INSUFFICIENT_DATA verdict, never a satisfied rule.
	results := make([]ScanResultItem, 0, len(universe))
	for _, symbol := range universe {
		if ctx.Err() != nil {
			mu.Lock()
			record.Results = results
			mu.Unlock()
			return // watchdog records the terminal state
		}
		bars := barsBySymbol[symbol]
		item := ScanResultItem{Symbol: symbol, Verdict: string(pullback.InsufficientData), Reason: "MISSING_BARS", BarsCount: len(bars)}
		if len(bars) > 0 {
			item.FirstBarDate = bars[0].TradeDate
			item.LastBarDate = bars[len(bars)-1].TradeDate
		}
		result, err := pullback.Evaluate(symbol, asOf, bars)
		switch {
		case err != nil:
			// The strategy contract treats malformed inputs as hard errors;
			// the scan records a fixed identifier instead of raw error text.
			item.Verdict = "ERROR"
			item.Reason = "EVALUATION_FAILED"
		default:
			item.Verdict = string(result.Verdict)
			item.Reason = result.Reason
		}
		digest, err := m.store.SaveSnapshot(context.Background(), record.ID, symbol, bars)
		if err != nil {
			terminal(ScanFailed, ScanFailurePersistence)
			return
		}
		item.SnapshotSHA256 = digest
		results = append(results, item)
		if len(results)%250 == 0 {
			mu.Lock()
			record.Results = results
			record.Progress.SymbolsEvaluated = len(results)
			err = persist()
			mu.Unlock()
			if err != nil {
				terminal(ScanFailed, ScanFailurePersistence)
				return
			}
		}
	}
	mu.Lock()
	record.Results = results
	record.Progress.SymbolsEvaluated = len(results)
	mu.Unlock()
	terminal(ScanCompleted, "")
}

func summarizeScan(results []ScanResultItem) *ScanSummary {
	summary := &ScanSummary{Total: len(results)}
	for _, item := range results {
		switch item.Verdict {
		case string(pullback.Selected):
			summary.Selected++
		case string(pullback.Rejected):
			summary.Rejected++
		case string(pullback.InsufficientData):
			summary.Insufficient++
		case "ERROR":
			summary.Errored++
		}
	}
	return summary
}

func (m *ScanManager) Get(ctx context.Context, id string) (ScanRecord, error) {
	if m == nil {
		return ScanRecord{}, errors.New("scan manager is nil")
	}
	return m.store.Get(ctx, id)
}

func (m *ScanManager) List(ctx context.Context) ([]ScanRecord, error) {
	if m == nil {
		return nil, errors.New("scan manager is nil")
	}
	return m.store.List(ctx)
}

func (m *ScanManager) Cancel(ctx context.Context, id string) error {
	if m == nil {
		return errors.New("scan manager is nil")
	}
	if ctx == nil {
		return errors.New("cancel context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	cancel, ok := m.active[id]
	if ok {
		cancel()
	}
	m.mu.Unlock()
	if !ok {
		return ErrScanNotActive
	}
	return nil
}

// GetSnapshot returns the exact observed input bars for one symbol's verdict.
// Snapshots of failed or cancelled scans remain readable; an unfinished scan
// has no committed snapshot surface yet.
func (m *ScanManager) GetSnapshot(ctx context.Context, scanID, symbol string) ([]technical.HistoricalBar, error) {
	if m == nil {
		return nil, errors.New("scan manager is nil")
	}
	record, err := m.store.Get(ctx, scanID)
	if err != nil {
		return nil, err
	}
	if record.State == ScanPending || record.State == ScanRunning {
		return nil, ErrScanNotTerminal
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	for _, item := range record.Results {
		if item.Symbol == symbol {
			return m.store.GetSnapshot(ctx, scanID, symbol, item.SnapshotSHA256)
		}
	}
	return nil, ErrScanSymbolNotFound
}

// SetScanTimeout configures the deadline for future scans. Call once during
// initialization before concurrent Submit calls; zero is not allowed.
func (m *ScanManager) SetScanTimeout(timeout time.Duration) error {
	if m == nil || timeout <= 0 {
		return errors.New("scan timeout must be positive")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.active) != 0 {
		return errors.New("cannot change timeout while a scan is active")
	}
	m.scanTimeout = timeout
	return nil
}

// CloseContext requests shutdown and waits for active scans to exit.
func (m *ScanManager) CloseContext(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("close context is nil")
	}
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.cancel()
		m.closeDone = make(chan struct{})
		done := m.closeDone
		go func() { m.wg.Wait(); close(done) }()
	}
	done := m.closeDone
	m.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
