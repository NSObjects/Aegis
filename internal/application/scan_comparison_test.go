package application

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

func comparisonBars(base float64, lastClose float64) []technical.HistoricalBar {
	bars := make([]technical.HistoricalBar, 65)
	for i := range bars {
		price := base + float64(i)*0.5
		bars[i] = technical.HistoricalBar{TradeDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("20060102"), Open: price, High: price + 1, Low: price - 1, Close: price, Volume: 1000, Amount: 100000}
	}
	bars[64].Open, bars[64].High, bars[64].Low, bars[64].Close = lastClose-1, lastClose+1, lastClose-2, lastClose
	return bars
}

func flatBars() []technical.HistoricalBar {
	bars := make([]technical.HistoricalBar, 65)
	for i := range bars {
		bars[i] = technical.HistoricalBar{TradeDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("20060102"), Open: 100, High: 101, Low: 99, Close: 100, Volume: 1000, Amount: 100000}
	}
	return bars
}

func obsFactors(bars []technical.HistoricalBar) []technical.PITAdjustmentFactor {
	out := make([]technical.PITAdjustmentFactor, len(bars))
	for i, b := range bars {
		out[i] = technical.PITAdjustmentFactor{TradeDate: b.TradeDate, Factor: 1}
	}
	return out
}

// completedComparisonScan writes one COMPLETED scan whose universe covers the
// three comparison cases: two selected candidates with different pullback
// depths, one rejected trend with a committed snapshot, and one symbol whose
// observation never validated (no snapshot).
func completedComparisonScan(t *testing.T, store *FileScanStore, deep, shallow string) ScanRecord {
	t.Helper()
	symbols := []string{"000001.SZ", deep, shallow, "601398.SH"}
	bars := map[string][]technical.HistoricalBar{
		deep: comparisonBars(100, 127.5), shallow: comparisonBars(100, 129),
		"000001.SZ": flatBars(), // flat trend rejects with TREND_NOT_UP but stays observable
	}
	asOf := bars[deep][len(bars[deep])-1].TradeDate
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
	// Snapshots must follow universe order; only the three observable symbols persist.
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

func sameTestFactors(bars []technical.HistoricalBar) []pullback.AdjustmentFactor {
	out := make([]pullback.AdjustmentFactor, len(bars))
	for i, b := range bars {
		out[i] = pullback.AdjustmentFactor{TradeDate: b.TradeDate, Factor: 1}
	}
	return out
}

func TestScanComparisonRanksDeterministicallyAndStatesAbsence(t *testing.T) {
	store, err := NewFileScanStore(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	record := completedComparisonScan(t, store, "600036.SH", "600519.SH")
	manager, err := NewScanManager(store, &scanStub{})
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := manager.Comparison(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.ComparisonVersion != pullback.ComparisonVersion || comparison.RankingRule != pullback.RankingRule || comparison.AsOf != record.AsOf {
		t.Fatalf("comparison identity wrong: %+v", comparison)
	}
	if len(comparison.Ranked) != 2 {
		t.Fatalf("expected two ranked candidates: %+v", comparison.Ranked)
	}
	// The deeper pullback (600036.SH, close 127.5) must rank above the shallow one.
	if comparison.Ranked[0].Symbol != "600036.SH" || comparison.Ranked[1].Symbol != "600519.SH" {
		t.Fatalf("ranking order wrong: %+v", comparison.Ranked)
	}
	if comparison.Ranked[0].Rank != 1 || comparison.Ranked[1].Rank != 2 {
		t.Fatalf("ranks not sequential: %+v", comparison.Ranked)
	}
	if comparison.Ranked[0].Metrics.CloseOverMA20 > comparison.Ranked[1].Metrics.CloseOverMA20 {
		t.Fatalf("pullback proximity must lead: %+v", comparison.Ranked)
	}
	for _, row := range comparison.Ranked {
		if row.Source.SHA256 == "" || row.Source.BarsCount != 65 {
			t.Fatalf("source not anchored: %+v", row)
		}
	}
	notes := map[string]string{}
	for _, row := range comparison.Incomparable {
		notes[row.Symbol] = row.Note
	}
	if notes["000001.SZ"] != "NOT_SELECTED" {
		t.Fatalf("rejected trend must state NOT_SELECTED: %+v", comparison.Incomparable)
	}
	if notes["601398.SH"] != "NO_COMMITTED_SNAPSHOT" {
		t.Fatalf("missing observation must be explicit: %+v", comparison.Incomparable)
	}
	// Determinism: rebuilding over the same store yields the identical view.
	again, err := manager.Comparison(record.ID)
	if err != nil || len(again.Ranked) != 2 || again.Ranked[0] != comparison.Ranked[0] || again.Ranked[1] != comparison.Ranked[1] {
		t.Fatalf("comparison not deterministic: %+v vs %+v (%v)", again.Ranked, comparison.Ranked, err)
	}
}

func TestScanComparisonFailsClosed(t *testing.T) {
	store, err := NewFileScanStore(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewScanManager(store, &scanStub{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Comparison("0123456789abcdef0123456789abcdee"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing scan err=%v", err)
	}
	// A still-running scan (no results yet) refuses comparison.
	now := time.Now().UTC()
	running := ScanRecord{ID: "0123456789abcdef0123456789abcdee", AsOf: "20260306", StrategyVersion: pullback.StrategyVersion, State: ScanRunning, UniverseSource: "USER_SUPPLIED", Symbols: []string{"600519.SH"}, CreatedAt: now, UpdatedAt: now}
	if err := store.Save(running); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Comparison(running.ID); !errors.Is(err, ErrScanNotCompleted) {
		t.Fatalf("unfinished err=%v", err)
	}
}
