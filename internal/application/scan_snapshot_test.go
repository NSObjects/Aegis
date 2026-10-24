package application

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "sync"
 "testing"
 "time"

 "Aegis/internal/capability/technical"
 "Aegis/internal/strategy/pullback"
)

// normalScanObservation builds a valid RAW observation map shaped exactly like
// a real technical tool result so ObserveHistorical validates it end to end.
// The 65-bar rising series ends with a pullback close on the requested date.
func normalScanObservation(symbol, asOf string) map[string]any {
 bars := make([]technical.HistoricalBar, 65)
 day := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
 for i := range bars {
  price := 100 + float64(i)*0.5
  date := day.AddDate(0, 0, i).Format("20060102")
  bars[i] = technical.HistoricalBar{TradeDate: date, Open: price, High: price + 1, Low: price - 1, Close: price, Volume: 1000, Amount: 100000}
 }
 last := len(bars) - 1
 bars[last].Open = 129
 bars[last].High = 130
 bars[last].Low = 128
 bars[last].Close = 129
 factors := make([]technical.PITAdjustmentFactor, len(bars))
 for i, b := range bars {
  factors[i] = technical.PITAdjustmentFactor{TradeDate: b.TradeDate, Factor: 1}
 }
 return map[string]any{
  "status": "NORMAL", "symbol": symbol, "requested_as_of": asOf, "effective_as_of": bars[last].TradeDate,
  "historical_ohlcv": map[string]any{"version": 1, "price_basis": "RAW", "bars": bars, "pit_adjustment_factors": factors},
 }
}

// observedScanStub answers NORMAL for chosen symbols and NO_DATA otherwise.
type observedScanStub struct {
 mu      sync.Mutex
 normal  map[string]bool
 started chan struct{}
 release chan struct{}
}

func (s *observedScanStub) Research(ctx context.Context, req technical.Request) (map[string]any, error) {
 select {
 case s.started <- struct{}{}:
 default:
 }
 select {
 case <-s.release:
 case <-ctx.Done():
  return nil, ctx.Err()
 }
 s.mu.Lock()
 ok := s.normal[req.Symbol]
 s.mu.Unlock()
 if ok {
  return normalScanObservation(req.Symbol, req.AsOf), nil
 }
 return map[string]any{"status": "NO_DATA"}, nil
}

func waitScanDone(t *testing.T, m *ScanManager, id string) ScanRecord {
 t.Helper()
 deadline := time.After(5 * time.Second)
 for {
  rec, err := m.Get(id)
  if err != nil {
   t.Fatal(err)
  }
  if rec.State == ScanCompleted || rec.State == ScanFailed {
   return rec
  }
  select {
  case <-deadline:
   t.Fatal("scan never reached terminal state")
  case <-time.After(5 * time.Millisecond):
  }
 }
}

func TestScanPersistsPerSymbolInputSnapshots(t *testing.T) {
 store, err := NewFileScanStore(filepath.Join(t.TempDir(), "private"))
 if err != nil {
  t.Fatal(err)
 }
 stub := &observedScanStub{normal: map[string]bool{"600519.SH": true}, started: make(chan struct{}, 2), release: make(chan struct{})}
 manager, err := NewScanManager(store, stub)
 if err != nil {
  t.Fatal(err)
 }
 submitted, err := manager.Submit(context.Background(), "20261022", "USER_SUPPLIED", []string{"600519.SH", "000001.SZ"})
 if err != nil {
  t.Fatal(err)
 }
 <-stub.started
 // While RUNNING there is no committed snapshot surface yet.
 if _, err := manager.GetSnapshot(submitted.ID, "600519.SH"); !errors.Is(err, ErrScanNotCompleted) {
  t.Fatalf("running scan snapshot err=%v", err)
 }
 close(stub.release)
 final := waitScanDone(t, manager, submitted.ID)
 if final.State != ScanCompleted {
  t.Fatalf("state=%s failure=%s", final.State, final.FailureCode)
 }
 if len(final.Snapshots) != 1 {
  t.Fatalf("snapshot refs=%+v", final.Snapshots)
 }
 ref := final.Snapshots[0]
 if ref.Symbol != "600519.SH" || ref.BarsCount != 65 || ref.EffectiveAsOf != "20261022" || len(ref.SHA256) != 64 {
  t.Fatalf("snapshot ref=%+v", ref)
 }
 observed, err := manager.GetSnapshot(final.ID, "600519.SH")
 if err != nil {
  t.Fatal(err)
 }
 if observed.Symbol != "600519.SH" || observed.PriceBasis != "RAW" || len(observed.Bars) != 65 || len(observed.PITFactors) != 65 {
  t.Fatalf("snapshot round-trip drifted: %+v", observed)
 }
 // The observed symbol wins SELECTED on this fixture; the NO_DATA symbol
 // keeps an INSUFFICIENT_DATA verdict and no snapshot at all.
 for _, result := range final.Results {
  if result.Symbol == "600519.SH" && result.Verdict != pullback.Selected {
   t.Fatalf("observed symbol verdict=%+v", result)
  }
  if result.Symbol == "000001.SZ" && result.Verdict != pullback.InsufficientData {
   t.Fatalf("no-data symbol verdict=%+v", result)
  }
 }
 if _, err := manager.GetSnapshot(final.ID, "000001.SZ"); !errors.Is(err, ErrScanSnapshotNotFound) {
  t.Fatalf("missing snapshot err=%v", err)
 }
 // Reopen proves snapshots survive restarts alongside their records.
 reopened, err := NewFileScanStore(store.dir)
 if err != nil {
  t.Fatal(err)
 }
 reloaded, err := reopened.Get(final.ID)
 if err != nil || len(reloaded.Snapshots) != 1 {
  t.Fatalf("reopen snapshots=%+v err=%v", reloaded.Snapshots, err)
 }
 again, err := NewScanManager(reopened, stub)
 if err != nil {
  t.Fatal(err)
 }
 if _, err := again.GetSnapshot(final.ID, "600519.SH"); err != nil {
  t.Fatalf("snapshot unreadable after restart: %v", err)
 }
}

func TestScanSnapshotDigestFailsClosed(t *testing.T) {
 store, err := NewFileScanStore(filepath.Join(t.TempDir(), "private"))
 if err != nil {
  t.Fatal(err)
 }
 const scanID = "0123456789abcdef0123456789abcdef"
 obs := technical.HistoricalObservation{Symbol: "600519.SH", RequestedAsOf: "20261022", EffectiveAsOf: "20261022", PriceBasis: "RAW", Bars: []technical.HistoricalBar{{TradeDate: "20261022", Open: 10, High: 11, Low: 9, Close: 10, Volume: 1, Amount: 1}}}
 digest, err := store.SaveSnapshot(scanID, obs)
 if err != nil {
  t.Fatal(err)
 }
 loaded, err := store.GetSnapshot(scanID, "600519.SH", digest)
 if err != nil || len(loaded.Bars) != 1 {
  t.Fatalf("round-trip err=%v bars=%d", err, len(loaded.Bars))
 }
 if _, err := store.GetSnapshot(scanID, "600519.SH", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err == nil {
  t.Fatal("digest mismatch accepted")
 }
 if _, err := store.GetSnapshot(scanID, "000001.SZ", digest); !errors.Is(err, os.ErrNotExist) {
  t.Fatalf("missing symbol snapshot err=%v", err)
 }
 bad := obs
 bad.Bars = []technical.HistoricalBar{{TradeDate: "20261021", Open: 10, High: 11, Low: 9, Close: 10, Volume: 1, Amount: 1}}
 if _, err := store.SaveSnapshot(scanID, bad); err == nil {
  t.Fatal("stale observation accepted")
 }
}

func TestValidateScanSnapshotReferences(t *testing.T) {
 now := time.Now().UTC()
 base := func(mutate func(*ScanRecord)) ScanRecord {
  r := ScanRecord{ID: "0123456789abcdef0123456789abcdef", AsOf: "20261022", StrategyVersion: pullback.StrategyVersion, State: ScanCompleted, UniverseSource: "USER_SUPPLIED", Symbols: []string{"000001.SZ", "600519.SH"}, CreatedAt: now, UpdatedAt: now,
   Results: []pullback.Result{
    {Symbol: "000001.SZ", AsOf: "20261022", Version: pullback.StrategyVersion, Verdict: pullback.Rejected, Reason: "TREND_NOT_UP"},
    {Symbol: "600519.SH", AsOf: "20261022", Version: pullback.StrategyVersion, Verdict: pullback.Selected, Reason: "TREND_PULLBACK_V1"},
   }}
  if mutate != nil {
   mutate(&r)
  }
  r.InputSnapshotSHA256 = ScanInputSnapshotSHA256(r)
  return r
 }
 const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
 ref := ScanSnapshotRef{Symbol: "600519.SH", SHA256: sha, BarsCount: 65, EffectiveAsOf: "20261022"}
 if err := validateScanRecord(base(func(r *ScanRecord) { r.Snapshots = []ScanSnapshotRef{ref} })); err != nil {
  t.Fatalf("aligned snapshot rejected: %v", err)
 }
 if err := validateScanRecord(base(func(r *ScanRecord) {
  outside := ref
  outside.Symbol = "300750.SZ"
  r.Snapshots = []ScanSnapshotRef{outside}
 })); err == nil {
  t.Fatal("out-of-universe snapshot accepted")
 }
 if err := validateScanRecord(base(func(r *ScanRecord) {
  r.Snapshots = []ScanSnapshotRef{{Symbol: "600519.SH", SHA256: sha, BarsCount: 1, EffectiveAsOf: "20261022"}, {Symbol: "000001.SZ", SHA256: sha, BarsCount: 1, EffectiveAsOf: "20261022"}}
 })); err == nil {
  t.Fatal("unordered snapshots accepted")
 }
 if err := validateScanRecord(base(func(r *ScanRecord) {
  future := ref
  future.EffectiveAsOf = "20261023"
  r.Snapshots = []ScanSnapshotRef{future}
 })); err == nil {
  t.Fatal("future effective date accepted")
 }
 if err := validateScanRecord(base(func(r *ScanRecord) {
  short := ref
  short.SHA256 = "abc"
  r.Snapshots = []ScanSnapshotRef{short}
 })); err == nil {
  t.Fatal("malformed digest accepted")
 }
 failed := base(func(r *ScanRecord) {
  r.State = ScanFailed
  r.FailureCode = "SCAN_EXECUTION_FAILED"
  r.Results = nil
  r.Snapshots = []ScanSnapshotRef{ref}
 })
 if err := validateScanRecord(failed); err == nil {
  t.Fatal("failed scan with snapshots accepted")
 }
}
