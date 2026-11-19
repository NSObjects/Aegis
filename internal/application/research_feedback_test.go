package application

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

func feedbackFixture(t *testing.T) (pullback.ValidationInput, ResearchHypothesis, HypothesisObservation) {
	t.Helper()
	bars := comparisonBars(100, 129)
	const symbol = "600519.SH"
	snapshot := func(bars []technical.HistoricalBar) pullback.PITSnapshot {
		result := pullback.PITSnapshot{StrategyVersion: pullback.StrategyVersion, CalendarSource: "fixture:calendar",
			UniverseSource: "fixture:universe", Members: []pullback.PITUniverseEntry{{Symbol: symbol}}}
		for _, bar := range bars {
			closeAt, err := hypothesisMarketClose(bar.TradeDate)
			if err != nil {
				t.Fatal(err)
			}
			known := closeAt.Add(time.Hour)
			factor := 1.0
			result.Sessions = append(result.Sessions, bar.TradeDate)
			result.Members[0].Rows = append(result.Members[0].Rows, pullback.PITRow{
				Session: bar.TradeDate, State: "TRADED", KnownAt: known, Bar: &bar,
				Factor: &factor, FactorKnownAt: &known,
			})
			result.Cutoff = known.Add(time.Hour)
		}
		result.CalendarKnownAt, result.UniverseKnownAt = result.Cutoff, result.Cutoff
		return result
	}
	first, second := snapshot(bars[:63]), snapshot(bars)
	testDay := second.Cutoff.AddDate(0, 0, 1)
	testThrough := testDay.Format("20060102")
	negative := false
	input := pullback.ValidationInput{
		Backtest: pullback.BacktestInput{
			Policy: pullback.BacktestPolicy{Shares: 100, HoldSessions: 1}, Signals: []pullback.PITSnapshot{first, second},
			Sessions: []pullback.OpeningSession{{Date: bars[62].TradeDate}, {Date: bars[63].TradeDate},
				{Date: bars[64].TradeDate}, {Date: testThrough}},
		},
		Folds: []pullback.ValidationFold{{TrainFrom: bars[62].TradeDate, TrainThrough: bars[63].TradeDate,
			TestFrom: bars[64].TradeDate, TestThrough: testThrough}},
		Outcomes: []pullback.CandidateOutcome{{Date: bars[64].TradeDate, Symbol: symbol, Positive: &negative,
			KnownAt: testDay, Source: "fixture:independent-outcome"}},
	}
	closePrice := 90.0
	return input, ResearchHypothesis{Symbol: symbol, AsOf: second.Cutoff,
			HypothesisDraft: HypothesisDraft{CloseRule: &HypothesisCloseRule{TradeDate: testThrough, Operator: "GTE", Threshold: 100}}},
		HypothesisObservation{AsOf: testDay.AddDate(0, 0, 1), Close: &closePrice, Reason: "PRICE_CRITERION", Status: "REFUTED", TradeDate: testThrough}
}

func TestResearchFeedbackSeparatesEvidenceAndRejectsInvalidTiming(t *testing.T) {
	input, item, observed := feedbackFixture(t)
	record, err := evaluateResearchFeedback(item, observed, input, observed.AsOf)
	if err != nil || record.Direction != "REFUTED" || record.Selection != "FALSE_POSITIVE" ||
		record.DataQuality != "EVALUABLE" || record.Timing != "ARCHIVE_CUTOFF_CHECKED" ||
		record.Outcome == nil || record.Outcome.Source != "fixture:independent-outcome" || len(record.ArchiveSHA256) != 64 {
		t.Fatalf("feedback lost evidence: %+v, %v", record, err)
	}
	tooEarly := item
	tooEarly.AsOf = item.AsOf.Add(-time.Minute)
	if _, err := evaluateResearchFeedback(tooEarly, observed, input, observed.AsOf); !errors.Is(err, ErrFeedbackUnavailable) {
		t.Fatalf("late snapshot was admitted to original research: %v", err)
	}

	input.Outcomes[0].KnownAt = observed.AsOf
	late, err := evaluateResearchFeedback(item, observed, input, observed.AsOf)
	if err != nil || late.Timing != "LABEL_AFTER_TEST_WINDOW" || late.Selection != "UNASSESSED" || late.Outcome != nil {
		t.Fatalf("late label leaked into selection verdict: %+v, %v", late, err)
	}
	input.Backtest.Signals[1].Members[0].Rows = nil
	missing, err := evaluateResearchFeedback(item, observed, input, observed.AsOf)
	if err != nil || missing.DataQuality != "MISSING_TRADING_SESSION_DATA" || missing.Selection != "UNASSESSED" {
		t.Fatalf("missing bars interpreted as misclassification: %+v, %v", missing, err)
	}
}

func TestResearchFeedbackAppendsToMemoryAndRespectsRevocation(t *testing.T) {
	input, item, observed := feedbackFixture(t)
	dir := filepath.Join(t.TempDir(), "hypotheses")
	store, err := NewFileHypothesisStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	origin := RunRecord{
		ID: strings.Repeat("a", 32), State: RunCompleted, Symbol: item.Symbol, RequestedAsOf: item.AsOf,
		Artifact: json.RawMessage(`{"persisted":"original"}`),
	}
	draft := item.HypothesisDraft
	draft.Statement, draft.Invalidation, draft.VerificationRule = "收盘价达到阈值", "低于阈值", "核验指定日"
	draft.ExpiresAt = observed.AsOf.Add(time.Hour)
	hypothesis, _, err := store.Add(origin, draft, item.AsOf)
	if err != nil {
		t.Fatal(err)
	}
	followup := origin
	followup.ID, followup.RequestedAsOf = strings.Repeat("b", 32), observed.AsOf
	followup.HistoricalOHLCV = &technical.HistoricalObservation{
		Symbol: item.Symbol, PriceBasis: "RAW", EffectiveAsOf: observed.TradeDate,
		Bars: []technical.HistoricalBar{{TradeDate: observed.TradeDate, Close: *observed.Close}},
	}
	if _, _, err := store.Observe(origin.ID, hypothesis.ID, followup); err != nil {
		t.Fatal(err)
	}
	now := observed.AsOf.Add(time.Hour)
	entry, _, err := store.AddMemory(MemorySource{SourceRunID: origin.ID, HypothesisID: hypothesis.ID, ObservationRunID: followup.ID}, now)
	if err != nil {
		t.Fatal(err)
	}
	saved, created, err := store.AddResearchFeedback(entry.ID, input, now)
	if err != nil || !created {
		t.Fatalf("feedback creation failed: %+v %v", saved, err)
	}
	store, err = NewFileHypothesisStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	retry, created, err := store.AddResearchFeedback(entry.ID, input, now.Add(time.Hour))
	if err != nil || created || retry.ArchiveSHA256 != saved.ArchiveSHA256 || !retry.RecordedAt.Equal(saved.RecordedAt) {
		t.Fatalf("duplicate changed original feedback: %+v, %v", retry, err)
	}
	memories, err := store.SearchMemory(MemoryFilter{})
	if err != nil || len(memories.Memories) != 1 || len(memories.Memories[0].Feedback) != 1 ||
		memories.Memories[0].Outcome != entry.Outcome || memories.Memories[0].Thesis != entry.Thesis {
		t.Fatalf("memory history corrupted: %+v, %v", memories, err)
	}
	if _, err := store.RevokeMemory(entry.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AddResearchFeedback(entry.ID, input, now); !errors.Is(err, ErrMemoryRevoked) {
		t.Fatalf("revoked entry accepted feedback: %v", err)
	}
}
