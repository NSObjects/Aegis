package application

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

func replayScanRecord(t *testing.T, store *FileScanStore) ScanRecord {
	t.Helper()
	bars := map[string][]technical.HistoricalBar{
		"000001.SZ": flatBars(),
		"600519.SH": comparisonBars(100, 129),
	}
	symbols := []string{"000001.SZ", "600519.SH", "601398.SH"}
	asOf := bars["600519.SH"][64].TradeDate
	now := time.Now().UTC()
	record := ScanRecord{ID: "0123456789abcdef0123456789abcdef", AsOf: asOf, StrategyVersion: pullback.StrategyVersion, State: ScanCompleted, UniverseSource: "USER_SUPPLIED", Symbols: symbols, CreatedAt: now, UpdatedAt: now}
	for _, symbol := range symbols {
		b := bars[symbol]
		if b == nil {
			record.Results = append(record.Results, pullback.Result{Symbol: symbol, AsOf: asOf, Version: pullback.StrategyVersion, Verdict: pullback.InsufficientData, Reason: "NO_VALID_HISTORICAL_OBSERVATION"})
			continue
		}
		verdict, err := pullback.Evaluate(symbol, asOf, b, sameTestFactors(b))
		if err != nil {
			t.Fatal(err)
		}
		record.Results = append(record.Results, verdict)
	}
	for _, symbol := range symbols {
		b := bars[symbol]
		if b == nil {
			continue
		}
		o := technical.HistoricalObservation{Symbol: symbol, RequestedAsOf: asOf, EffectiveAsOf: asOf, PriceBasis: "RAW", Bars: b, PITFactors: obsFactors(b)}
		digest, err := store.SaveSnapshot(record.ID, o)
		if err != nil {
			t.Fatal(err)
		}
		record.Snapshots = append(record.Snapshots, ScanSnapshotRef{Symbol: symbol, SHA256: digest, BarsCount: len(b), EffectiveAsOf: asOf})
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestScanReplayReproducesPersistedResults(t *testing.T) {
	store, err := NewFileScanStore(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	record := replayScanRecord(t, store)
	manager, err := NewScanManager(store, &scanStub{})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := manager.ReplayScan(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayable || !replay.Matches {
		t.Fatalf("identical inputs must replay: %+v", replay)
	}
	if len(replay.Rows) != 2 || len(replay.Unreplayable) != 1 {
		t.Fatalf("rows/unreplayable wrong: %+v", replay)
	}
	if replay.Unreplayable[0].Symbol != "601398.SH" || replay.Unreplayable[0].Reason != "NO_COMMITTED_SNAPSHOT" {
		t.Fatalf("unreplayable must be explicit: %+v", replay.Unreplayable)
	}
	for _, row := range replay.Rows {
		if !row.Match || row.Replayed != row.Persisted || row.Source.SHA256 == "" {
			t.Fatalf("row mismatch: %+v", row)
		}
	}
	// Determinism: repeated replays over the same store agree.
	again, err := manager.ReplayScan(record.ID)
	if err != nil || !again.Matches {
		t.Fatalf("replay not deterministic: %+v %v", again, err)
	}
}

func TestScanReplayDetectsTamperedResults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	store, err := NewFileScanStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	record := replayScanRecord(t, store)
	// Hand-edit the persisted JSON the way a corrupted record would appear.
	// The input fingerprint only covers inputs, so the forged results pass
	// record validation while contradicting the persisted snapshots; the
	// replay must catch exactly this gap.
	tampered := record
	tampered.Results[1] = pullback.Result{Symbol: "600519.SH", AsOf: record.AsOf, Version: pullback.StrategyVersion, Verdict: pullback.Rejected, Reason: "HAND_EDITED"}
	tampered.InputSnapshotSHA256 = ScanInputSnapshotSHA256(tampered)
	raw, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, record.ID+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewScanManager(store, &scanStub{})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := manager.ReplayScan(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Matches {
		t.Fatalf("tampered result must not match: %+v", replay)
	}
	found := false
	for _, row := range replay.Rows {
		if row.Symbol == "600519.SH" && !row.Match {
			found = true
		}
	}
	if !found {
		t.Fatalf("mismatched row missing: %+v", replay.Rows)
	}
}

func TestScanReplayFailsClosed(t *testing.T) {
	store, err := NewFileScanStore(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewScanManager(store, &scanStub{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReplayScan("0123456789abcdef0123456789abcdee"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing scan err=%v", err)
	}
	// A running scan refuses replay.
	now := time.Now().UTC()
	running := ScanRecord{ID: "0123456789abcdef0123456789abcdee", AsOf: "20260306", StrategyVersion: pullback.StrategyVersion, State: ScanRunning, UniverseSource: "USER_SUPPLIED", Symbols: []string{"600519.SH"}, CreatedAt: now, UpdatedAt: now}
	if err := store.Save(running); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReplayScan(running.ID); !errors.Is(err, ErrScanNotCompleted) {
		t.Fatalf("unfinished err=%v", err)
	}
}
