package application

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
)

// fakeScanSource is a deterministic in-memory BatchMarketSource.
type fakeScanSource struct {
	mu       sync.Mutex
	days     []string
	listed   []string
	byDay    map[string][]technical.BatchBar
	failDay  string
	blockDay string
	block    chan struct{}
	hits     int
}

func (f *fakeScanSource) TradingDays(_ context.Context, asOf string, n int) ([]string, error) {
	if len(f.days) > n {
		return f.days[len(f.days)-n:], nil
	}
	return f.days, nil
}
func (f *fakeScanSource) ListedSymbols(_ context.Context) ([]string, error) {
	return f.listed, nil
}
func (f *fakeScanSource) DailyBars(ctx context.Context, day string) ([]technical.BatchBar, error) {
	f.mu.Lock()
	f.hits++
	f.mu.Unlock()
	if f.failDay == day {
		return nil, errors.New("market data provider unavailable")
	}
	if f.blockDay == day {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.byDay[day], nil
}

// scanCalendarDays yields len(count) consecutive calendar dates, which the
// strategy only requires to be strictly increasing and parseable.
func scanCalendarDays(count int) []string {
	days := make([]string, count)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range days {
		days[i] = start.AddDate(0, 0, i).Format("20060102")
	}
	return days
}

// pullbackBatchBars builds the observed SELECTED shape for one symbol: a
// steady uptrend whose final session pulls back to the 20-day average.
func pullbackBatchBars(symbol string, days []string) []technical.BatchBar {
	bars := make([]technical.BatchBar, len(days))
	for i, day := range days {
		price := 100 + float64(i)*0.5
		bars[i] = technical.BatchBar{Symbol: symbol, TradeDate: day, Open: price, High: price + 1, Low: price - 1, Close: price, Volume: 1000, Amount: 100000}
	}
	last := len(bars) - 1
	// Final close pulls back into [MA20, MA20*1.03]: roughly four points
	// below the prior close of a steadily rising series.
	bars[last].Open = 140
	bars[last].High = 141
	bars[last].Low = 139
	bars[last].Close = 140
	return bars
}

func trendOnlyBatchBars(symbol string, days []string) []technical.BatchBar {
	bars := pullbackBatchBars(symbol, days)
	// Remove the pullback: the close keeps rising away from MA20.
	last := len(bars) - 1
	rising := 100 + float64(last)*0.5 + 1
	bars[last] = technical.BatchBar{Symbol: symbol, TradeDate: days[last], Open: rising, High: rising + 1, Low: rising - 1, Close: rising, Volume: 1000, Amount: 100000}
	return bars
}

func newScanFixture(t *testing.T) (*fakeScanSource, []string) {
	t.Helper()
	days := scanCalendarDays(ScanLookbackTradingDays)
	asOf := days[len(days)-1]
	byDay := make(map[string][]technical.BatchBar)
	bySymbolDay := func(symbol string, bars []technical.BatchBar, dropLast bool) {
		if dropLast {
			bars = bars[:len(bars)-1]
		}
		for _, bar := range bars {
			byDay[bar.TradeDate] = append(byDay[bar.TradeDate], bar)
		}
	}
	bySymbolDay("600519.SH", pullbackBatchBars("600519.SH", days), false)
	// 000001.SZ is halted on the as-of date: no bar, which must remain
	// insufficient data rather than satisfying any rule.
	bySymbolDay("000001.SZ", pullbackBatchBars("000001.SZ", days), true)
	bySymbolDay("300001.SZ", pullbackBatchBars("300001.SZ", days), false)
	bySymbolDay("600000.SH", trendOnlyBatchBars("600000.SH", days), false)
	source := &fakeScanSource{days: days, listed: []string{"600519.SH", "000001.SZ", "300001.SZ", "600000.SH"}, byDay: byDay}
	return source, []string{asOf}
}

func newScanManagerForTest(t *testing.T, source *fakeScanSource) (*ScanManager, *FileScanStore) {
	t.Helper()
	store, err := NewFileScanStore(filepath.Join(t.TempDir(), "scans"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewScanManager(func(context.Context) (technical.BatchMarketSource, error) { return source, nil }, store, 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.CloseContext(context.Background()) })
	return manager, store
}

func waitScanTerminal(t *testing.T, m *ScanManager, id string) ScanRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec, err := m.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		switch rec.State {
		case ScanCompleted, ScanFailed, ScanCancelled:
			return rec
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("scan did not reach a terminal state")
	return ScanRecord{}
}

func TestScanRecordsVerdictsReasonsAndSnapshots(t *testing.T) {
	source, args := newScanFixture(t)
	manager, _ := newScanManagerForTest(t, source)
	submitted, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0]})
	if err != nil {
		t.Fatal(err)
	}
	rec := waitScanTerminal(t, manager, submitted.ID)
	if rec.State != ScanCompleted {
		t.Fatalf("state=%s failure=%s", rec.State, rec.FailureCode)
	}
	want := map[string]struct{ verdict, reason string }{
		"600519.SH": {string("SELECTED"), "TREND_PULLBACK_V1"},
		"000001.SZ": {"INSUFFICIENT_DATA", "MISSING_BARS"},
		"300001.SZ": {"REJECTED", "OUTSIDE_STOCK_UNIVERSE"},
		"600000.SH": {"REJECTED", "NOT_NEAR_MA20"},
	}
	if len(rec.Results) != len(want) {
		t.Fatalf("results=%d want %d", len(rec.Results), len(want))
	}
	for _, item := range rec.Results {
		expected, ok := want[item.Symbol]
		if !ok {
			t.Fatalf("unexpected symbol %s", item.Symbol)
		}
		if item.Verdict != expected.verdict || item.Reason != expected.reason {
			t.Fatalf("%s verdict=%s reason=%s want %+v", item.Symbol, item.Verdict, item.Reason, expected)
		}
		if len(item.SnapshotSHA256) != 64 {
			t.Fatalf("%s missing snapshot digest", item.Symbol)
		}
	}
	if rec.Summary == nil || rec.Summary.Total != 4 || rec.Summary.Selected != 1 || rec.Summary.Rejected != 2 || rec.Summary.Insufficient != 1 || rec.Summary.Errored != 0 {
		t.Fatalf("summary=%+v", rec.Summary)
	}
	bars, err := manager.GetSnapshot(context.Background(), submitted.ID, "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != ScanLookbackTradingDays || bars[len(bars)-1].TradeDate != args[0] {
		t.Fatalf("snapshot bars=%d last=%s", len(bars), bars[len(bars)-1].TradeDate)
	}
	halted, err := manager.GetSnapshot(context.Background(), submitted.ID, "000001.SZ")
	if err != nil || len(halted) != ScanLookbackTradingDays-1 {
		t.Fatalf("halted snapshot bars=%d err=%v", len(halted), err)
	}
}

func TestScanMissingDataNeverSatisfiesRule(t *testing.T) {
	// The halted fixture above already proves absence yields
	// INSUFFICIENT_DATA; this asserts the window shortfall path too.
	source, args := newScanFixture(t)
	manager, _ := newScanManagerForTest(t, source)
	submitted, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0], Symbols: []string{"000001.SZ"}})
	if err != nil {
		t.Fatal(err)
	}
	rec := waitScanTerminal(t, manager, submitted.ID)
	if rec.State != ScanCompleted || len(rec.Results) != 1 {
		t.Fatalf("state=%s results=%d failure=%s", rec.State, len(rec.Results), rec.FailureCode)
	}
	item := rec.Results[0]
	if item.Verdict != "INSUFFICIENT_DATA" || item.Reason != "MISSING_BARS" || item.LastBarDate >= args[0] {
		t.Fatalf("missing data treated as satisfying: %+v", item)
	}
}

func TestScanFailsClosedWhenMarketDataUnavailable(t *testing.T) {
	source, args := newScanFixture(t)
	source.failDay = source.days[45]
	manager, _ := newScanManagerForTest(t, source)
	submitted, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0]})
	if err != nil {
		t.Fatal(err)
	}
	rec := waitScanTerminal(t, manager, submitted.ID)
	if rec.State != ScanFailed || rec.FailureCode != ScanFailureData {
		t.Fatalf("state=%s failure=%s", rec.State, rec.FailureCode)
	}
	if rec.Progress.DataDaysFetched != 45 {
		t.Fatalf("progress days=%d want 45", rec.Progress.DataDaysFetched)
	}
	if rec.Summary != nil {
		t.Fatal("failed scan must not expose a summary")
	}
}

func TestScanRejectsNonTradingAsOf(t *testing.T) {
	source, args := newScanFixture(t)
	// Trim the final trading day so the calendar ends before the as-of date.
	source.days = source.days[:len(source.days)-1]
	manager, _ := newScanManagerForTest(t, source)
	submitted, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0]})
	if err != nil {
		t.Fatal(err)
	}
	rec := waitScanTerminal(t, manager, submitted.ID)
	if rec.State != ScanFailed || rec.FailureCode != ScanFailureNotTradingDay {
		t.Fatalf("state=%s failure=%s", rec.State, rec.FailureCode)
	}
}

func TestScanUnknownExplicitSymbolFailsClosed(t *testing.T) {
	source, args := newScanFixture(t)
	manager, _ := newScanManagerForTest(t, source)
	submitted, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0], Symbols: []string{"600519.SH", "999999.SH"}})
	if err != nil {
		t.Fatal(err)
	}
	rec := waitScanTerminal(t, manager, submitted.ID)
	if rec.State != ScanFailed || rec.FailureCode != ScanFailureUnknownSymbol {
		t.Fatalf("state=%s failure=%s", rec.State, rec.FailureCode)
	}
}

func TestScanCancelWhileFetching(t *testing.T) {
	source, args := newScanFixture(t)
	source.blockDay = source.days[1]
	source.block = make(chan struct{})
	manager, _ := newScanManagerForTest(t, source)
	submitted, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0]})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		source.mu.Lock()
		hits := source.hits
		source.mu.Unlock()
		if hits >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err := manager.Cancel(context.Background(), submitted.ID); err != nil {
		t.Fatal(err)
	}
	rec := waitScanTerminal(t, manager, submitted.ID)
	if rec.State != ScanCancelled {
		t.Fatalf("state=%s failure=%s", rec.State, rec.FailureCode)
	}
}

func TestScanReplayReproducesOutcomes(t *testing.T) {
	source, args := newScanFixture(t)
	manager, _ := newScanManagerForTest(t, source)
	first, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0]})
	if err != nil {
		t.Fatal(err)
	}
	firstFinal := waitScanTerminal(t, manager, first.ID)
	second, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0]})
	if err != nil {
		t.Fatal(err)
	}
	secondFinal := waitScanTerminal(t, manager, second.ID)
	if first.ID == second.ID {
		t.Fatal("distinct submissions must create distinct scans")
	}
	if len(firstFinal.Results) != len(secondFinal.Results) {
		t.Fatalf("result counts differ: %d vs %d", len(firstFinal.Results), len(secondFinal.Results))
	}
	for i := range firstFinal.Results {
		a, b := firstFinal.Results[i], secondFinal.Results[i]
		if a != b {
			t.Fatalf("replayed outcome drifted: %+v vs %+v", a, b)
		}
	}
}

func TestScanIdempotencyReuseAndConflict(t *testing.T) {
	source, args := newScanFixture(t)
	manager, _ := newScanManagerForTest(t, source)
	key := "scan-idem-key-1"
	first, err := manager.Submit(context.Background(), ScanRequest{IdempotencyKey: key, AsOf: args[0], Symbols: []string{"600519.SH"}})
	if err != nil {
		t.Fatal(err)
	}
	waitScanTerminal(t, manager, first.ID)
	reused, err := manager.Submit(context.Background(), ScanRequest{IdempotencyKey: key, AsOf: args[0], Symbols: []string{"600519.SH"}})
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != first.ID {
		t.Fatalf("idempotent reuse returned %s want %s", reused.ID, first.ID)
	}
	if _, err := manager.Submit(context.Background(), ScanRequest{IdempotencyKey: key, AsOf: args[0], Symbols: []string{"600000.SH"}}); !errors.Is(err, ErrScanIdempotencyConflict) {
		t.Fatalf("conflict err=%v", err)
	}
}

func TestScanSubmitValidatesRequest(t *testing.T) {
	source, args := newScanFixture(t)
	manager, _ := newScanManagerForTest(t, source)
	if _, err := manager.Submit(context.Background(), ScanRequest{AsOf: "2026-2-30"}); !errors.Is(err, ErrInvalidScanRequest) {
		t.Fatalf("invalid date err=%v", err)
	}
	if _, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0], Symbols: []string{"600abc.SH"}}); !errors.Is(err, ErrInvalidScanRequest) {
		t.Fatalf("invalid symbol err=%v", err)
	}
	if _, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0], Symbols: []string{"600519.SH", "600519.SH"}}); !errors.Is(err, ErrInvalidScanRequest) {
		t.Fatalf("duplicate symbol err=%v", err)
	}
}

func TestScanManagerReconcilesInterruptedScans(t *testing.T) {
	source, _ := newScanFixture(t)
	dir := t.TempDir()
	store, err := NewFileScanStore(filepath.Join(dir, "scans"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	started := now.Add(-time.Minute)
	stranded := ScanRecord{ID: strings.Repeat("b", 32), State: ScanRunning, RequestSnapshot: &ScanRequestSnapshot{SchemaVersion: 1, AsOf: "20260101", StrategyVersion: "trend-pullback-v1", UniversePolicy: "LISTED_SH_SZ", LookbackTradingDays: ScanLookbackTradingDays}, Progress: ScanProgress{DataDaysTotal: ScanLookbackTradingDays}, CreatedAt: now, StartedAt: &started, Events: []ScanEvent{{Seq: 1, Stage: "Scan", Status: string(ScanRunning), At: started}}}
	if err := store.Save(context.Background(), stranded); err != nil {
		t.Fatal(err)
	}
	manager, err := NewScanManager(func(context.Context) (technical.BatchMarketSource, error) { return source, nil }, store, 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.CloseContext(context.Background()) })
	rec, err := manager.Get(context.Background(), stranded.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != ScanFailed || rec.FailureCode != ScanFailureInterrupted {
		t.Fatalf("state=%s failure=%s", rec.State, rec.FailureCode)
	}
}

func TestScanSummaryMatchesPersistedResults(t *testing.T) {
	source, args := newScanFixture(t)
	manager, store := newScanManagerForTest(t, source)
	submitted, err := manager.Submit(context.Background(), ScanRequest{AsOf: args[0]})
	if err != nil {
		t.Fatal(err)
	}
	rec := waitScanTerminal(t, manager, submitted.ID)
	tampered := rec
	tampered.Summary.Selected = 99
	if err := store.Save(context.Background(), tampered); err == nil {
		t.Fatal("tampered summary accepted")
	}
}
