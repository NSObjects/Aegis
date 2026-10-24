package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
)

func monitorTestRule(kind MonitorKind, id string, threshold float64) MonitorRule {
	now := time.Now().UTC()
	return MonitorRule{ID: id, Symbol: "600519.SH", Kind: kind, Threshold: threshold, CooldownDays: 3, Enabled: true, CreatedAt: now, UpdatedAt: now}
}

func TestFileMonitorStoreRuleAndEventLifecycle(t *testing.T) {
	store, err := NewFileMonitorStore(filepath.Join(t.TempDir(), "monitor"))
	if err != nil {
		t.Fatal(err)
	}
	rule := monitorTestRule(MonitorPriceBelow, "0123456789abcdef", 1500)
	if err := store.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRule(rule.ID)
	if err != nil || got.Threshold != 1500 {
		t.Fatalf("rule roundtrip: %+v %v", got, err)
	}
	changed := rule
	changed.Kind = MonitorPriceAbove
	if err := store.SaveRule(changed); err == nil {
		t.Fatal("kind mutation accepted")
	}
	rules, err := store.ListRules()
	if err != nil || len(rules) != 1 {
		t.Fatalf("list: %+v %v", rules, err)
	}
	// Validation refusals.
	bad := monitorTestRule(MonitorPriceBelow, "bad-id", 1)
	if err := store.SaveRule(bad); err == nil {
		t.Fatal("bad id accepted")
	}
	if err := store.SaveRule(monitorTestRule(MonitorPriceBelow, "ffffffffffffffff", 0)); err == nil {
		t.Fatal("zero threshold accepted")
	}
	if err := store.SaveRule(monitorTestRule(MonitorAnnouncement, "ffffffffffffffff", 5)); err == nil {
		t.Fatal("announcement with threshold accepted")
	}
	// Events: first outcome of a day wins, duplicates are refused.
	now := time.Now().UTC()
	event := MonitorEvent{RuleID: rule.ID, Symbol: "600519.SH", Kind: MonitorPriceBelow, TradeDate: "20261023", Outcome: MonitorFired, Detail: "close=1499", CreatedAt: now}
	if err := store.SaveEvent(event); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEvent(event); !errors.Is(err, ErrMonitorEventExists) {
		t.Fatalf("duplicate event err=%v", err)
	}
	gotEvent, err := store.GetEvent(rule.ID, "20261023")
	if err != nil || gotEvent.Outcome != MonitorFired {
		t.Fatalf("event roundtrip: %+v %v", gotEvent, err)
	}
	second := event
	second.TradeDate = "20261024"
	second.Outcome = MonitorSourceDown
	second.Detail = "SOURCE_UNAVAILABLE"
	if err := store.SaveEvent(second); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListEvents(rule.ID)
	if err != nil || len(events) != 2 || events[0].TradeDate != "20261024" {
		t.Fatalf("events not newest first: %+v %v", events, err)
	}
	latest, err := store.LatestEvent(rule.ID)
	if err != nil || latest.TradeDate != "20261024" {
		t.Fatalf("latest: %+v %v", latest, err)
	}
	// Delete only rules, never silently events.
	if err := store.DeleteRule(rule.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetRule(rule.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted rule err=%v", err)
	}
}

type monitorSourceStub struct {
	obs       technical.HistoricalObservation
	obsOK     bool
	obsErr    error
	staleAsOf string // when set, observations carry this effective date
	anns      []technical.Announcement
	annErr    error
}

func (s *monitorSourceStub) Observation(_ context.Context, _, asOf string) (technical.HistoricalObservation, bool, error) {
	obs := s.obs
	if s.staleAsOf != "" {
		obs.EffectiveAsOf = s.staleAsOf
	} else {
		obs.EffectiveAsOf = asOf
	}
	obs.RequestedAsOf = asOf
	return obs, s.obsOK, s.obsErr
}
func (s *monitorSourceStub) Announcements(context.Context, string, string, string) ([]technical.Announcement, error) {
	return s.anns, s.annErr
}

func monitorBarsWithClose(closePrice, volume float64) technical.HistoricalObservation {
	bars := make([]technical.HistoricalBar, 10)
	for i := range bars {
		price := closePrice + float64(10-i)
		v := volume
		if i < 9 {
			v = 1000
		}
		bars[i] = technical.HistoricalBar{TradeDate: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("20060102"), Open: price, High: price + 1, Low: price - 1, Close: price, Volume: v, Amount: v * price}
	}
	bars[9].Close = closePrice
	return technical.HistoricalObservation{Symbol: "600519.SH", RequestedAsOf: "20261021", EffectiveAsOf: "20261021", PriceBasis: "RAW", Bars: bars, PITFactors: obsFactors(bars)}
}

func TestMonitorDetectsPriceConditions(t *testing.T) {
	store, err := NewFileMonitorStore(filepath.Join(t.TempDir(), "monitor"))
	if err != nil {
		t.Fatal(err)
	}
	below := monitorTestRule(MonitorPriceBelow, "0123456789abcdef", 1500)
	above := monitorTestRule(MonitorPriceAbove, "ffffffffffffffff", 2000)
	if err := store.SaveRule(below); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRule(above); err != nil {
		t.Fatal(err)
	}
	source := &monitorSourceStub{obs: monitorBarsWithClose(1499, 10000), obsOK: true}
	monitor, err := NewMonitor(store, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := monitor.Tick(context.Background(), "20261021"); err != nil {
		t.Fatal(err)
	}
	fired, err := store.GetEvent(below.ID, "20261021")
	if err != nil || fired.Outcome != MonitorFired || fired.Detail != "close=1499 threshold=1500" {
		t.Fatalf("below not fired: %+v %v", fired, err)
	}
	quiet, err := store.GetEvent(above.ID, "20261021")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quiet must not persist an event: %+v %v", quiet, err)
	}
	// Only the fired rule has an event; one per rule per day.
	events, _ := store.ListEvents(below.ID)
	if len(events) != 1 {
		t.Fatalf("duplicate outcomes: %+v", events)
	}
}

func TestMonitorVolumeSpikeAndAnnouncements(t *testing.T) {
	store, err := NewFileMonitorStore(filepath.Join(t.TempDir(), "monitor"))
	if err != nil {
		t.Fatal(err)
	}
	spike := monitorTestRule(MonitorVolumeSpike, "0123456789abcdef", 3)
	ann := monitorTestRule(MonitorAnnouncement, "ffffffffffffffff", 0)
	if err := store.SaveRule(spike); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRule(ann); err != nil {
		t.Fatal(err)
	}
	source := &monitorSourceStub{obs: monitorBarsWithClose(1500, 9000), obsOK: true, anns: []technical.Announcement{{AnnDate: "20261021", Title: "回购公告"}}}
	monitor, err := NewMonitor(store, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := monitor.Tick(context.Background(), "20261021"); err != nil {
		t.Fatal(err)
	}
	fired, err := store.GetEvent(spike.ID, "20261021")
	if err != nil || fired.Outcome != MonitorFired {
		t.Fatalf("spike not fired: %+v %v", fired, err)
	}
	annEvent, err := store.GetEvent(ann.ID, "20261021")
	if err != nil || annEvent.Outcome != MonitorFired || annEvent.Detail != "announcements=1 latest=回购公告" {
		t.Fatalf("announcement not fired: %+v %v", annEvent, err)
	}
}

func TestMonitorCooldownSuppressesRepeats(t *testing.T) {
	store, err := NewFileMonitorStore(filepath.Join(t.TempDir(), "monitor"))
	if err != nil {
		t.Fatal(err)
	}
	rule := monitorTestRule(MonitorPriceBelow, "0123456789abcdef", 1500)
	if err := store.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	source := &monitorSourceStub{obs: monitorBarsWithClose(1499, 1000), obsOK: true}
	monitor, err := NewMonitor(store, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := monitor.Tick(context.Background(), "20261021"); err != nil {
		t.Fatal(err)
	}
	// Two calendar days later, still inside the 3-day cooldown.
	if err := monitor.Tick(context.Background(), "20261023"); err != nil {
		t.Fatal(err)
	}
	suppressed, err := store.GetEvent(rule.ID, "20261023")
	if err != nil || suppressed.Outcome != MonitorSuppressed {
		t.Fatalf("cooldown not applied: %+v %v", suppressed, err)
	}
	// After the cooldown the rule fires again.
	if err := monitor.Tick(context.Background(), "20261026"); err != nil {
		t.Fatal(err)
	}
	refired, err := store.GetEvent(rule.ID, "20261026")
	if err != nil || refired.Outcome != MonitorFired {
		t.Fatalf("refire missing: %+v %v", refired, err)
	}
	// The same day never records twice even across ticks.
	events, _ := store.ListEvents(rule.ID)
	if len(events) != 3 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
}

func TestMonitorSourceOutageBecomesAlarmNotSilence(t *testing.T) {
	store, err := NewFileMonitorStore(filepath.Join(t.TempDir(), "monitor"))
	if err != nil {
		t.Fatal(err)
	}
	price := monitorTestRule(MonitorPriceBelow, "0123456789abcdef", 1500)
	ann := monitorTestRule(MonitorAnnouncement, "ffffffffffffffff", 0)
	if err := store.SaveRule(price); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRule(ann); err != nil {
		t.Fatal(err)
	}
	source := &monitorSourceStub{obsErr: errors.New("tushare down"), annErr: errors.New("tushare down")}
	monitor, err := NewMonitor(store, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := monitor.Tick(context.Background(), "20261021"); err != nil {
		t.Fatal(err)
	}
	priceEvent, err := store.GetEvent(price.ID, "20261021")
	if err != nil || priceEvent.Outcome != MonitorSourceDown {
		t.Fatalf("price outage not alarmed: %+v %v", priceEvent, err)
	}
	annEvent, err := store.GetEvent(ann.ID, "20261021")
	if err != nil || annEvent.Outcome != MonitorSourceDown {
		t.Fatalf("announcement outage not alarmed: %+v %v", annEvent, err)
	}
	// Recovery after the cooldown fires normally instead of claiming continuity;
	// the outage day never extends the cooldown.
	source.obsErr = nil
	source.obsOK = true
	source.obs = monitorBarsWithClose(1499, 1000)
	if err := monitor.Tick(context.Background(), "20261026"); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.GetEvent(price.ID, "20261026")
	if err != nil || recovered.Outcome != MonitorFired {
		t.Fatalf("recovery not fired: %+v %v", recovered, err)
	}
}

func TestMonitorStaleObservationIsQuietNotSignal(t *testing.T) {
	store, err := NewFileMonitorStore(filepath.Join(t.TempDir(), "monitor"))
	if err != nil {
		t.Fatal(err)
	}
	rule := monitorTestRule(MonitorPriceBelow, "0123456789abcdef", 1500)
	if err := store.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	source := &monitorSourceStub{obs: monitorBarsWithClose(100, 1000), obsOK: true, staleAsOf: "20261020"}
	monitor, err := NewMonitor(store, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := monitor.Tick(context.Background(), "20261021"); err != nil {
		t.Fatal(err)
	}
	// The stale fallback price (100) must not masquerade as a crash below 1500:
	// no event is recorded and no signal claimed.
	if _, err := store.GetEvent(rule.ID, "20261021"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale treated as signal: %v", err)
	}
}
