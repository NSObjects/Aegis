package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
)

func TestAutomaticResearchObservesEarlierHypothesesAndConversationRespectsRevocation(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileHypothesisStore(filepath.Join(t.TempDir(), "hypotheses"))
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Now().UTC().AddDate(0, 0, -5).Truncate(24 * time.Hour).Add(8 * time.Hour)
	tradeDate := asOf.AddDate(0, 0, 1).Format("20060102")
	source := RunRecord{ID: strings.Repeat("a", 32), State: RunCompleted, Symbol: "600519.SH",
		RequestedAsOf: asOf, Artifact: json.RawMessage(`{"source":true}`)}
	automatic := source
	automatic.ID = strings.Repeat("b", 32)
	automatic.RequestedAsOf = asOf.Add(24 * time.Hour)
	automatic.HistoricalOHLCV = &technical.HistoricalObservation{
		Symbol: source.Symbol, PriceBasis: "RAW", EffectiveAsOf: tradeDate,
		Bars: []technical.HistoricalBar{{TradeDate: tradeDate, Close: 99}},
	}
	draft := HypothesisDraft{
		Statement: "目标日的收盘价高于阈值", ExpiresAt: asOf.Add(48 * time.Hour),
		Invalidation: "收盘价低于阈值", VerificationRule: "目标日收盘价",
		CloseRule: &HypothesisCloseRule{TradeDate: tradeDate, Operator: "GTE", Threshold: 100},
	}
	item, _, err := store.Add(source, draft, asOf.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	draft.Statement = "后续报告之后录入的假设"
	late, _, err := store.Add(source, draft, automatic.RequestedAsOf.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	triggers, err := NewFileTriggerStore(filepath.Join(t.TempDir(), "triggers"))
	if err != nil {
		t.Fatal(err)
	}
	runs := &triggerRunsStub{records: map[string]RunRecord{automatic.ID: automatic}}
	router, err := NewTriggerRouter(triggers, runs, ModeDecision, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router.WithHypothesisStore(store)
	record := TriggerRecord{
		ID: "0123456789abcdef" + tradeDate[2:], RuleID: "0123456789abcdef", Symbol: source.Symbol,
		TradeDate: tradeDate, MonitorKind: MonitorAnnouncement, Route: RouteThesisReview,
		RunID: automatic.ID, State: ScheduleRunning, CreatedAt: asOf, UpdatedAt: asOf,
	}
	if err := triggers.Save(record); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := router.Advance(ctx); err != nil {
			t.Fatal(err)
		}
	}
	observations, err := store.Observations(source.ID, item.ID)
	if err != nil || len(observations) != 1 || observations[0].Status != "REFUTED" || observations[0].RunID != automatic.ID {
		t.Fatalf("automatic observation missing or duplicated: %+v, %v", observations, err)
	}
	if records, err := store.Observations(source.ID, late.ID); err != nil || len(records) != 0 {
		t.Fatalf("future annotation was evaluated: %+v, %v", records, err)
	}
	const reviewReason = "需核查推理是否忽略了成交条件"
	now := automatic.RequestedAsOf.Add(2 * time.Hour)
	review, _, err := store.AddReview(source.ID, item.ID, HypothesisReviewDraft{
		ObservationRunID: automatic.ID, Category: "REASONING_BIAS", Reason: reviewReason,
		Supporting: []HypothesisEvidence{{Reference: "后续报告", Detail: "目标日低于阈值"}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := store.AddMemory(MemorySource{
		SourceRunID: source.ID, HypothesisID: item.ID, ObservationRunID: automatic.ID, ReviewID: review.ID,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	args := ConversationMemoryArgs{Symbol: source.Symbol, Query: "目标日"}
	if early, err := store.ConversationMemory(ctx, args, automatic.RequestedAsOf.Add(-time.Minute)); err != nil ||
		len(early.Hypotheses) != 1 || early.Hypotheses[0].Latest != nil || len(early.Samples) != 0 {
		t.Fatalf("lookup included future evidence: %+v, %v", early, err)
	}
	result, err := store.ConversationMemory(ctx, args, now)
	if err != nil || len(result.Samples) != 1 || result.Samples[0].Review == nil ||
		result.Samples[0].Review.Reason != reviewReason || len(result.Hypotheses) != 1 || result.Hypotheses[0].SourceRunID != source.ID {
		t.Fatalf("lookup lost provenance or review: %+v, %v", result, err)
	}
	if _, err := store.RevokeMemory(memory.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	result, err = store.ConversationMemory(ctx, args, now.Add(2*time.Minute))
	if err != nil || len(result.Samples) != 0 || len(result.Hypotheses) != 1 || result.Hypotheses[0].Latest == nil {
		t.Fatalf("revocation did not hide just the curated sample: %+v, %v", result, err)
	}
	tools := (&ConversationResearchTools{runs: &conversationRunsStub{}}).WithResearchMemory(store)
	reads, err := tools.EinoReadTools()
	if err != nil {
		t.Fatal(err)
	}
	for _, read := range reads {
		info, err := read.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "ReadResearchMemory" {
			reply, err := read.InvokableRun(ctx, `{"symbol":"600519.SH","q":"目标日"}`)
			if err != nil || !strings.Contains(reply, automatic.ID) || strings.Contains(reply, reviewReason) {
				t.Fatalf("unexpected tool reply: %s, %v", reply, err)
			}
			return
		}
	}
	t.Fatal("ReadResearchMemory tool is missing")
}
