package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
)

func newScanStoreForTest(t *testing.T) *FileScanStore {
	t.Helper()
	store, err := NewFileScanStore(filepath.Join(t.TempDir(), "scans"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func completedScanRecord() ScanRecord {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	started := now.Add(-time.Minute)
	return ScanRecord{
		ID: strings.Repeat("c", 32), State: ScanCompleted,
		RequestSnapshot: &ScanRequestSnapshot{SchemaVersion: 1, AsOf: "20261020", StrategyVersion: "trend-pullback-v1", UniversePolicy: "LISTED_SH_SZ", LookbackTradingDays: ScanLookbackTradingDays},
		Progress:        ScanProgress{SymbolsTotal: 1, SymbolsEvaluated: 1, DataDaysTotal: ScanLookbackTradingDays, DataDaysFetched: ScanLookbackTradingDays},
		Results:         []ScanResultItem{{Symbol: "600519.SH", Verdict: "SELECTED", Reason: "TREND_PULLBACK_V1", BarsCount: 90, FirstBarDate: "20260720", LastBarDate: "20261020", SnapshotSHA256: strings.Repeat("a", 64)}},
		Summary:         &ScanSummary{Total: 1, Selected: 1},
		CreatedAt:       started, StartedAt: &started, FinishedAt: &now,
		Events: []ScanEvent{
			{Seq: 1, Stage: "Scan", Status: string(ScanPending), At: started},
			{Seq: 2, Stage: "Scan", Status: string(ScanRunning), At: started.Add(time.Second)},
			{Seq: 3, Stage: "Scan", Status: string(ScanCompleted), At: now},
		},
	}
}

func TestFileScanStoreRoundTrip(t *testing.T) {
	store := newScanStoreForTest(t)
	record := completedScanRecord()
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != ScanCompleted || loaded.Summary == nil || len(loaded.Results) != 1 {
		t.Fatalf("round-trip drift: %+v", loaded)
	}
	listed, err := store.List(context.Background())
	if err != nil || len(listed) != 1 || listed[0].ID != record.ID {
		t.Fatalf("list=%d err=%v", len(listed), err)
	}
}

func TestFileScanStoreRejectsUnknownFieldsAndTampering(t *testing.T) {
	store := newScanStoreForTest(t)
	record := completedScanRecord()
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	path := store.recordPath(record.ID)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var withExtra map[string]any
	if err := json.Unmarshal(raw, &withExtra); err != nil {
		t.Fatal(err)
	}
	withExtra["injected"] = true
	tampered, err := json.Marshal(withExtra)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tampered, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), record.ID); err == nil {
		t.Fatal("unknown field accepted")
	}

	// Restore and tamper the persisted summary against its results.
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Summary.Rejected = 5
	if err := store.Save(context.Background(), loaded); err == nil {
		t.Fatal("summary drift accepted")
	}
}

func TestFileScanStoreSnapshotIntegrity(t *testing.T) {
	store := newScanStoreForTest(t)
	record := completedScanRecord()
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	bars := []technical.HistoricalBar{
		{TradeDate: "20261019", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100, Amount: 1000},
		{TradeDate: "20261020", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100, Amount: 1000},
	}
	digest, err := store.SaveSnapshot(context.Background(), record.ID, "600519.SH", bars)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetSnapshot(context.Background(), record.ID, "600519.SH", digest)
	if err != nil || len(loaded) != len(bars) {
		t.Fatalf("snapshot bars=%d err=%v", len(loaded), err)
	}
	// An empty series remains readable: no data is itself reviewable input.
	emptyDigest, err := store.SaveSnapshot(context.Background(), record.ID, "000001.SZ", nil)
	if err != nil {
		t.Fatal(err)
	}
	if empty, err := store.GetSnapshot(context.Background(), record.ID, "000001.SZ", emptyDigest); err != nil || len(empty) != 0 {
		t.Fatalf("empty snapshot err=%v bars=%d", err, len(empty))
	}
	// A digest mismatch must fail closed.
	if _, err := store.GetSnapshot(context.Background(), record.ID, "600519.SH", strings.Repeat("b", 64)); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	// File tampering must be detected through re-projection.
	snapshotPath := store.snapshotPath(record.ID, "600519.SH")
	raw, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["price_basis"] = "ADJUSTED"
	if raw2, err := json.Marshal(doc); err == nil {
		if err := os.WriteFile(snapshotPath, raw2, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.GetSnapshot(context.Background(), record.ID, "600519.SH", digest); err == nil {
		t.Fatal("tampered snapshot accepted")
	}
}

func TestFileScanStoreRejectsUnsafePathsAndDirs(t *testing.T) {
	store := newScanStoreForTest(t)
	if _, err := store.SaveSnapshot(context.Background(), strings.Repeat("c", 32), "../escape.SH", nil); err == nil {
		t.Fatal("unsafe snapshot symbol accepted")
	}
	if _, err := store.Get(context.Background(), "../../etc"); err == nil {
		t.Fatal("unsafe scan id accepted")
	}
	unsafe := filepath.Join(t.TempDir(), "unsafe-scans")
	if err := os.Mkdir(unsafe, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileScanStore(unsafe); err == nil {
		t.Fatal("public scan directory accepted")
	}
}

func TestFileScanStoreRecordValidation(t *testing.T) {
	store := newScanStoreForTest(t)
	cases := map[string]func(*ScanRecord){
		"bad state":          func(r *ScanRecord) { r.State = "PAUSED" },
		"missing snapshot":   func(r *ScanRecord) { r.RequestSnapshot = nil },
		"bad strategy":       func(r *ScanRecord) { r.RequestSnapshot.StrategyVersion = "mystery-v9" },
		"bad verdict":        func(r *ScanRecord) { r.Results[0].Verdict = "MAYBE" },
		"bad reason":         func(r *ScanRecord) { r.Results[0].Reason = "LOOKED_FINE" },
		"unsorted results":   func(r *ScanRecord) { r.Results = append(r.Results, ScanResultItem{Symbol: "000001.SZ", Verdict: "REJECTED", Reason: "TREND_NOT_UP", SnapshotSHA256: strings.Repeat("d", 64)}) },
		"failure on success": func(r *ScanRecord) { r.FailureCode = ScanFailureData },
	}
	for name, mutate := range cases {
		record := completedScanRecord()
		mutate(&record)
		if err := store.Save(context.Background(), record); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}
