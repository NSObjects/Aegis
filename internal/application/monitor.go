package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"Aegis/internal/capability/technical"
)

// MonitorDataSource supplies the market and announcement inputs for one
// monitoring pass. Every failure is surfaced per rule; the monitor never
// turns a source outage into "no event".
type MonitorDataSource interface {
	Observation(ctx context.Context, symbol, asOf string) (technical.HistoricalObservation, bool, error)
	Announcements(ctx context.Context, symbol, start, end string) ([]technical.Announcement, error)
}

type tushareMonitorSource struct {
	researcher technical.Researcher
	client     *technical.TushareClient
}

func (s *tushareMonitorSource) Observation(ctx context.Context, symbol, asOf string) (technical.HistoricalObservation, bool, error) {
	return technical.ObserveHistorical(ctx, s.researcher, technical.Request{Symbol: symbol, AsOf: asOf})
}

func (s *tushareMonitorSource) Announcements(ctx context.Context, symbol, start, end string) ([]technical.Announcement, error) {
	return technical.ListAnnouncements(ctx, s.client, symbol, start, end)
}

// NewTushareMonitorSource wires the existing capability into the monitor.
func NewTushareMonitorSource(researcher technical.Researcher, client *technical.TushareClient) (MonitorDataSource, error) {
	if researcher == nil || client == nil {
		return nil, errors.New("monitor source requires researcher and client")
	}
	return &tushareMonitorSource{researcher: researcher, client: client}, nil
}

// Monitor evaluates persisted rules over provider data on one trading day.
// It is fully deterministic: no model confidence, no inferred events, and
// source outages become SOURCE_UNAVAILABLE alarms instead of silence.
type Monitor struct {
	store  *FileMonitorStore
	source MonitorDataSource
	now    func() time.Time
}

func NewMonitor(store *FileMonitorStore, source MonitorDataSource) (*Monitor, error) {
	if store == nil || source == nil {
		return nil, errors.New("monitor requires store and source")
	}
	return &Monitor{store: store, source: source, now: func() time.Time { return time.Now().UTC() }}, nil
}

// Tick runs every enabled rule for one trading day and persists one outcome
// per rule. Rules that fired within their cooldown are recorded as
// COOLDOWN_SUPPRESSED, keeping the suppression itself auditable.
func (m *Monitor) Tick(ctx context.Context, tradeDate string) error {
	if _, err := time.Parse("20060102", tradeDate); err != nil {
		return errors.New("invalid monitor trade date")
	}
	rules, err := m.store.ListRules()
	if err != nil {
		return fmt.Errorf("list monitor rules: %w", err)
	}
	for _, rule := range rules {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !rule.Enabled {
			continue
		}
		outcome, detail, evalErr := m.evaluateRule(ctx, rule, tradeDate)
		if evalErr != nil {
			log.Printf("AEGIS_MONITOR_SOURCE_UNAVAILABLE rule=%s symbol=%s err=%v", rule.ID, rule.Symbol, evalErr)
			outcome, detail = MonitorSourceDown, "SOURCE_UNAVAILABLE"
		}
		if outcome == MonitorQuiet {
			continue
		}
		if outcome == MonitorFired {
			suppressed, err := m.withinCooldown(rule, tradeDate)
			if err != nil {
				return err
			}
			if suppressed {
				outcome, detail = MonitorSuppressed, "COOLDOWN_SUPPRESSED"
			}
		}
		event := MonitorEvent{RuleID: rule.ID, Symbol: rule.Symbol, Kind: rule.Kind, TradeDate: tradeDate, Outcome: outcome, Detail: detail, CreatedAt: m.now().UTC()}
		if err := m.store.SaveEvent(event); err != nil && !errors.Is(err, ErrMonitorEventExists) {
			return fmt.Errorf("save monitor event: %w", err)
		}
	}
	return nil
}

// withinCooldown reports whether the rule fired recently enough that a new
// FIRED outcome must be suppressed. Cooldown counts calendar days from the
// most recent FIRED event only: suppressions and outages never extend it.
func (m *Monitor) withinCooldown(rule MonitorRule, tradeDate string) (bool, error) {
	if rule.CooldownDays <= 0 {
		return false, nil
	}
	events, err := m.store.ListEvents(rule.ID)
	if err != nil {
		return false, err
	}
	var lastFired string
	for _, event := range events {
		if event.Outcome != MonitorFired {
			continue
		}
		lastFired = event.TradeDate
		break
	}
	if lastFired == "" {
		return false, nil
	}
	last, err := time.Parse("20060102", lastFired)
	if err != nil {
		return false, err
	}
	now, err := time.Parse("20060102", tradeDate)
	if err != nil {
		return false, err
	}
	return now.Sub(last).Hours() < float64(rule.CooldownDays)*24, nil
}

func (m *Monitor) evaluateRule(ctx context.Context, rule MonitorRule, tradeDate string) (MonitorOutcome, string, error) {
	switch rule.Kind {
	case MonitorPriceBelow, MonitorPriceAbove:
		obs, ok, err := m.source.Observation(ctx, rule.Symbol, tradeDate)
		if err != nil {
			return "", "", err
		}
		if !ok {
			return MonitorQuiet, "", nil
		}
		if obs.EffectiveAsOf != tradeDate {
			// Stale or halted data is not a price signal and not an outage:
			// record quiet with an explicit note.
			return MonitorQuiet, "", nil
		}
		if err := validateMonitorBars(obs); err != nil {
			return "", "", err
		}
		closePrice := obs.Bars[len(obs.Bars)-1].Close
		fired := closePrice < rule.Threshold
		if rule.Kind == MonitorPriceAbove {
			fired = closePrice > rule.Threshold
		}
		if !fired {
			return MonitorQuiet, "", nil
		}
		return MonitorFired, "close=" + formatThreshold(closePrice) + " threshold=" + formatThreshold(rule.Threshold), nil
	case MonitorVolumeSpike:
		obs, ok, err := m.source.Observation(ctx, rule.Symbol, tradeDate)
		if err != nil {
			return "", "", err
		}
		if !ok {
			return MonitorQuiet, "", nil
		}
		if obs.EffectiveAsOf != tradeDate {
			return MonitorQuiet, "", nil
		}
		if err := validateMonitorBars(obs); err != nil {
			return "", "", err
		}
		if len(obs.Bars) < 6 {
			return MonitorQuiet, "", nil
		}
		bars := obs.Bars
		last := bars[len(bars)-1]
		mean := 0.0
		for _, b := range bars[len(bars)-6 : len(bars)-1] {
			mean += b.Volume
		}
		mean /= 5
		if mean <= 0 {
			return MonitorQuiet, "", nil
		}
		ratio := last.Volume / mean
		if ratio < rule.Threshold {
			return MonitorQuiet, "", nil
		}
		return MonitorFired, "volume=" + formatThreshold(last.Volume) + " ratio=" + formatThreshold(ratio), nil
	case MonitorAnnouncement:
		end, err := time.Parse("20060102", tradeDate)
		if err != nil {
			return "", "", err
		}
		start := end.AddDate(0, 0, -2).Format("20060102")
		anns, err := m.source.Announcements(ctx, rule.Symbol, start, tradeDate)
		if err != nil {
			return "", "", err
		}
		if len(anns) == 0 {
			return MonitorQuiet, "", nil
		}
		detail := "announcements=" + strconv.Itoa(len(anns))
		if len(anns) > 0 && len(anns[0].Title) <= 200 {
			detail += " latest=" + anns[0].Title
		}
		return MonitorFired, detail, nil
	}
	return "", "", errors.New("unknown monitor kind")
}

// validateMonitorBars keeps the monitor's arithmetic on the same validated
// shape the pullback strategy consumes: positive volumes, sane OHLC.
func validateMonitorBars(obs technical.HistoricalObservation) error {
	if len(obs.Bars) == 0 {
		return errors.New("empty observation bars")
	}
	last := obs.Bars[len(obs.Bars)-1]
	if last.Close <= 0 || last.Volume < 0 {
		return errors.New("invalid last bar")
	}
	for _, b := range obs.Bars {
		if b.Close <= 0 || b.Volume < 0 {
			return errors.New("invalid bar")
		}
	}
	return nil
}

// TickVisible is the synchronous pass for tests and manual inspection.
func (m *Monitor) TickVisible(ctx context.Context, tradeDate string) error {
	return m.Tick(ctx, tradeDate)
}
