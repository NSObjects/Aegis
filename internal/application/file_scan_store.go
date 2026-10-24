package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

const maxScanRecordBytes int64 = 64 << 20

// scanSnapshotFile is the durable, per-symbol input snapshot. Bars are the
// exact raw observed inputs behind one verdict, never a derived series.
type scanSnapshotFile struct {
	SchemaVersion int                      `json:"schema_version"`
	Symbol        string                   `json:"symbol"`
	AsOf          string                   `json:"as_of"`
	PriceBasis    string                   `json:"price_basis"`
	Bars          []technical.HistoricalBar `json:"bars"`
}

// FileScanStore is a single-process, durable scan repository following the
// atomic-replacement JSON conventions of FileRunStore. Main records live at
// <dir>/<id>.json; per-symbol input snapshots live at
// <dir>/<id>/snapshots/<symbol>.json so a full-market scan never approaches
// the single-record size limit.
type FileScanStore struct {
	dir string
	mu  sync.Mutex
}

func NewFileScanStore(dir string) (*FileScanStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("scan store directory is required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create scan directory: %w", err)
	}
	stat, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, errors.New("scan store path is not a directory")
	}
	// Snapshots and records are market data the operator fetched; keep the
	// store private like every other durable Aegis artifact directory.
	if stat.Mode().Perm()&0077 != 0 {
		return nil, errors.New("scan store directory must be private (0700)")
	}
	return &FileScanStore{dir: dir}, nil
}

var scanReasonAllowlist = map[string]bool{
	"TREND_PULLBACK_V1": true, "TREND_NOT_UP": true, "NOT_NEAR_MA20": true,
	"OUTSIDE_STOCK_UNIVERSE": true, "HALTED_OR_NO_VOLUME": true,
	"MISSING_BARS": true, "EVALUATION_FAILED": true,
}

func checkScanRecord(record ScanRecord) error {
	if !validRunID(record.ID) {
		return errors.New("invalid strategy scan ID")
	}
	if (record.IdempotencyDigest == "") != (record.RequestFingerprint == "") {
		return errors.New("incomplete scan idempotency metadata")
	}
	if record.IdempotencyDigest != "" {
		for _, value := range []string{record.IdempotencyDigest, record.RequestFingerprint} {
			decoded, err := hex.DecodeString(value)
			if err != nil || len(value) != 64 || len(decoded) != 32 || value != strings.ToLower(value) {
				return errors.New("invalid scan idempotency digest")
			}
		}
	}
	snapshot := record.RequestSnapshot
	if snapshot == nil {
		return errors.New("scan request snapshot is required")
	}
	if snapshot.SchemaVersion != 1 {
		return errors.New("unsupported scan request snapshot version")
	}
	if err := ValidateScanAsOf(snapshot.AsOf); err != nil {
		return errors.New("invalid scan as-of date")
	}
	if snapshot.StrategyVersion != pullback.StrategyVersion {
		return errors.New("unsupported scan strategy version")
	}
	if snapshot.UniversePolicy != "LISTED_SH_SZ" {
		return errors.New("invalid scan universe policy")
	}
	if snapshot.LookbackTradingDays < 65 || snapshot.LookbackTradingDays > 250 {
		return errors.New("invalid scan lookback window")
	}
	if len(snapshot.Symbols) > maxExplicitScanSymbols {
		return errors.New("scan symbol subset exceeds limit")
	}
	for i, symbol := range snapshot.Symbols {
		if err := ValidateScanSymbol(symbol); err != nil {
			return errors.New("invalid scan snapshot symbol")
		}
		if i > 0 && symbol <= snapshot.Symbols[i-1] {
			return errors.New("scan snapshot symbols must be sorted and unique")
		}
	}
	if record.CreatedAt.IsZero() {
		return errors.New("scan creation time is required")
	}
	switch record.State {
	case ScanPending, ScanRunning, ScanCompleted, ScanFailed, ScanCancelled:
	default:
		return errors.New("invalid strategy scan state")
	}
	if record.State == ScanPending && (record.StartedAt != nil || record.FinishedAt != nil || len(record.Results) > 0 || record.Summary != nil) {
		return errors.New("pending scan cannot carry execution data")
	}
	if record.State == ScanRunning && (record.StartedAt == nil || record.FinishedAt != nil || record.Summary != nil) {
		return errors.New("running scan requires a start time and no summary")
	}
	if record.State == ScanCompleted {
		if record.FinishedAt == nil || record.Summary == nil || record.FailureCode != "" {
			return errors.New("completed scan requires summary and finish time")
		}
		if err := checkScanSummary(record); err != nil {
			return err
		}
	} else if record.Summary != nil {
		return errors.New("unfinished scan must not expose a summary")
	}
	if record.State != ScanFailed && record.FailureCode != "" {
		return errors.New("non-failed scan cannot carry a failure code")
	}
	switch record.FailureCode {
	case "", ScanFailureCredential, ScanFailureData, ScanFailureNotTradingDay,
		ScanFailureUnknownSymbol, ScanFailurePersistence, ScanFailureTimeout, ScanFailureInterrupted:
	default:
		return errors.New("invalid scan failure code")
	}
	if (record.State == ScanFailed || record.State == ScanCancelled) && record.FinishedAt == nil {
		return errors.New("terminal scan requires completion time")
	}
	p := record.Progress
	if p.SymbolsTotal < 0 || p.SymbolsEvaluated < 0 || p.DataDaysTotal < 0 || p.DataDaysFetched < 0 ||
		p.SymbolsEvaluated > p.SymbolsTotal || p.DataDaysFetched > p.DataDaysTotal {
		return errors.New("invalid scan progress")
	}
	// The full data window is only guaranteed once the scan completes: a
	// RUNNING record persists before the calendar fetch declares the window.
	if record.State == ScanCompleted && p.DataDaysTotal != snapshot.LookbackTradingDays {
		return errors.New("completed scan must declare the full data window")
	}
	if len(record.Results) > 20000 {
		return errors.New("scan result limit exceeded")
	}
	for i, item := range record.Results {
		if err := ValidateScanSymbol(item.Symbol); err != nil {
			return errors.New("invalid scan result symbol")
		}
		if i > 0 && item.Symbol <= record.Results[i-1].Symbol {
			return errors.New("scan results must be sorted by symbol")
		}
		switch item.Verdict {
		case string(pullback.Selected), string(pullback.Rejected), string(pullback.InsufficientData), "ERROR":
		default:
			return errors.New("invalid scan result verdict")
		}
		if !scanReasonAllowlist[item.Reason] {
			return errors.New("invalid scan result reason")
		}
		if item.BarsCount < 0 || len(item.SnapshotSHA256) != 64 {
			return errors.New("invalid scan result snapshot reference")
		}
		if decoded, err := hex.DecodeString(item.SnapshotSHA256); err != nil || len(decoded) != 32 || item.SnapshotSHA256 != strings.ToLower(item.SnapshotSHA256) {
			return errors.New("invalid scan result snapshot digest")
		}
	}
	for i, ev := range record.Events {
		if ev.Seq != uint64(i+1) || ev.At.IsZero() {
			return errors.New("invalid scan event sequence")
		}
		switch ev.Stage {
		case "Scan", "Universe", "MarketData":
		default:
			return errors.New("invalid scan event stage")
		}
		switch ev.Status {
		case string(ScanPending), string(ScanRunning), string(ScanCompleted), string(ScanFailed), string(ScanCancelled), "DONE":
		default:
			return errors.New("invalid scan event status")
		}
	}
	return nil
}

// checkScanSummary recomputes the verdict counts from the persisted results
// so the stored aggregate cannot drift from the itemized outcomes.
func checkScanSummary(record ScanRecord) error {
	recomputed := summarizeScan(record.Results)
	if *recomputed != *record.Summary {
		return errors.New("scan summary does not match results")
	}
	if record.Summary.Total != len(record.Results) {
		return errors.New("scan summary total mismatch")
	}
	if record.Progress.SymbolsTotal != record.Summary.Total || record.Progress.SymbolsEvaluated != record.Summary.Total {
		return errors.New("completed scan progress must cover every symbol")
	}
	return nil
}

func (s *FileScanStore) Save(ctx context.Context, record ScanRecord) error {
	if s == nil {
		return errors.New("scan store is nil")
	}
	if ctx == nil {
		return errors.New("save context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkScanRecord(record); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode strategy scan: %w", err)
	}
	if int64(len(payload)) > maxScanRecordBytes {
		return errors.New("strategy scan snapshot exceeds size limit")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return atomicWriteFile(s.dir, record.ID+".json", payload)
}

// atomicWriteFile durably replaces one JSON file via temp file, fsync and
// rename, following the FileRunStore crash-recovery conventions.
func atomicWriteFile(dir, name string, payload []byte) error {
	tmp, err := os.CreateTemp(dir, ".scan-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	path := tmp.Name()
	defer os.Remove(path)
	defer tmp.Close()
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		return fmt.Errorf("write scan file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync scan file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(path, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("replace scan file: %w", err)
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync scan directory: %w", err)
	}
	return nil
}

func (s *FileScanStore) recordPath(id string) string { return filepath.Join(s.dir, id+".json") }

func (s *FileScanStore) Get(ctx context.Context, id string) (ScanRecord, error) {
	if s == nil {
		return ScanRecord{}, errors.New("scan store is nil")
	}
	if ctx == nil {
		return ScanRecord{}, errors.New("get context is nil")
	}
	if err := ctx.Err(); err != nil {
		return ScanRecord{}, err
	}
	if !validRunID(id) {
		return ScanRecord{}, errors.New("invalid strategy scan ID")
	}
	f, err := os.Open(s.recordPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return ScanRecord{}, ErrScanNotFound
	}
	if err != nil {
		return ScanRecord{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ScanRecord{}, err
	}
	if info.Size() > maxScanRecordBytes {
		return ScanRecord{}, errors.New("strategy scan file exceeds limit")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxScanRecordBytes+1))
	if err != nil {
		return ScanRecord{}, err
	}
	if int64(len(raw)) > maxScanRecordBytes {
		return ScanRecord{}, errors.New("strategy scan file exceeds limit")
	}
	var result ScanRecord
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&result); err != nil {
		return ScanRecord{}, fmt.Errorf("decode strategy scan: %w", err)
	}
	var trailing any
	if err = dec.Decode(&trailing); err != io.EOF {
		return ScanRecord{}, errors.New("strategy scan file contains trailing JSON")
	}
	if result.ID != id {
		return ScanRecord{}, errors.New("strategy scan file ID mismatch")
	}
	if err := checkScanRecord(result); err != nil {
		return ScanRecord{}, err
	}
	result.RequestSnapshot = detachScanSnapshot(result.RequestSnapshot)
	result.Results = append([]ScanResultItem(nil), result.Results...)
	result.Events = append([]ScanEvent(nil), result.Events...)
	return result, nil
}

func (s *FileScanStore) List(ctx context.Context) ([]ScanRecord, error) {
	if s == nil {
		return nil, errors.New("scan store is nil")
	}
	if ctx == nil {
		return nil, errors.New("list context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	records := make([]ScanRecord, 0, len(entries))
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		record, err := s.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("read strategy scan %s: %w", e.Name(), err)
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records, nil
}

func (s *FileScanStore) snapshotPath(scanID, symbol string) string {
	return filepath.Join(s.dir, scanID, "snapshots", symbol+".json")
}

func (s *FileScanStore) SaveSnapshot(ctx context.Context, scanID, symbol string, bars []technical.HistoricalBar) (string, error) {
	if s == nil {
		return "", errors.New("scan store is nil")
	}
	if ctx == nil {
		return "", errors.New("save snapshot context is nil")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !validRunID(scanID) {
		return "", errors.New("invalid strategy scan ID")
	}
	if err := ValidateScanSymbol(symbol); err != nil {
		return "", errors.New("invalid snapshot symbol")
	}
	if len(bars) > 512 {
		return "", errors.New("snapshot bar limit exceeded")
	}
	file := scanSnapshotFile{
		SchemaVersion: 1, Symbol: symbol, PriceBasis: "RAW",
		Bars: append(make([]technical.HistoricalBar, 0, len(bars)), bars...),
	}
	payload, err := json.Marshal(file)
	if err != nil {
		return "", fmt.Errorf("encode scan snapshot: %w", err)
	}
	dir := filepath.Dir(s.snapshotPath(scanID, symbol))
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create scan snapshot directory: %w", err)
	}
	if err := atomicWriteFile(dir, symbol+".json", payload); err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (s *FileScanStore) GetSnapshot(ctx context.Context, scanID, symbol, expectedSHA256 string) ([]technical.HistoricalBar, error) {
	if s == nil {
		return nil, errors.New("scan store is nil")
	}
	if ctx == nil {
		return nil, errors.New("get snapshot context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validRunID(scanID) {
		return nil, errors.New("invalid strategy scan ID")
	}
	if err := ValidateScanSymbol(symbol); err != nil {
		return nil, errors.New("invalid snapshot symbol")
	}
	raw, err := os.ReadFile(s.snapshotPath(scanID, symbol))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrScanSymbolNotFound
	}
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > 1<<20 {
		return nil, errors.New("scan snapshot file exceeds limit")
	}
	var file scanSnapshotFile
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("decode scan snapshot: %w", err)
	}
	if file.SchemaVersion != 1 || file.Symbol != symbol || file.PriceBasis != "RAW" {
		return nil, errors.New("invalid scan snapshot metadata")
	}
	// Re-project the decoded snapshot and compare digests so any drift
	// between the stored bytes and the referenced checksum fails closed.
	reprojected, err := json.Marshal(file)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(reprojected)
	if hex.EncodeToString(sum[:]) != expectedSHA256 {
		return nil, errors.New("scan snapshot checksum mismatch")
	}
	return append([]technical.HistoricalBar(nil), file.Bars...), nil
}
