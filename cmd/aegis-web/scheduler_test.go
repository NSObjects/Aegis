package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Aegis/internal/application"
	"Aegis/internal/capability/technical"
	"Aegis/internal/portfolio"
)

type scheduledRuns struct {
	request application.ResearchRequest
	record  application.RunRecord
}

func (s *scheduledRuns) Submit(_ context.Context, req application.ResearchRequest) (application.RunRecord, error) {
	s.request = req
	return s.record, nil
}

func (s *scheduledRuns) Get(context.Context, string) (application.RunRecord, error) {
	return s.record, nil
}

func TestScheduledResearchUsesUnknownAccountAndAddsFollowup(t *testing.T) {
	ctx := context.Background()
	store, err := application.NewFileHypothesisStore(filepath.Join(t.TempDir(), "hypotheses"))
	if err != nil {
		t.Fatal(err)
	}
	const symbol = "600519.SH"
	asOf := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
	source := application.RunRecord{
		ID: strings.Repeat("a", 32), State: application.RunCompleted, Symbol: symbol,
		RequestedAsOf: asOf, Artifact: json.RawMessage(`{"source":true}`),
	}
	item, _, err := store.Add(source, application.HypothesisDraft{
		Statement: "目标日达到阈值", ExpiresAt: asOf.Add(48 * time.Hour),
		Invalidation: "低于阈值", VerificationRule: "目标日收盘价",
		CloseRule: &application.HypothesisCloseRule{TradeDate: "20261117", Operator: "GTE", Threshold: 100},
	}, asOf)
	if err != nil {
		t.Fatal(err)
	}
	followup := source
	followup.ID = strings.Repeat("b", 32)
	followup.RequestedAsOf = asOf.Add(24 * time.Hour)
	followup.HistoricalOHLCV = &technical.HistoricalObservation{
		Symbol: symbol, PriceBasis: "RAW", EffectiveAsOf: "20261117",
		Bars: []technical.HistoricalBar{{TradeDate: "20261117", Close: 105}},
	}
	runs := &scheduledRuns{record: followup}
	runner := &webScheduleRunner{runs: runs, mode: application.ModePortfolio, lookback: 7 * 24 * time.Hour}
	task := application.ScheduleTask{ID: "0123456789abcdef", Symbol: symbol, Kind: application.ScheduleKindResearch}
	id, err := runner.RunResearch(ctx, task, "20261117")
	if err != nil || id != followup.ID || runs.request.Portfolio == nil || runs.request.Portfolio.State != portfolio.StateNotProvided {
		t.Fatalf("schedule must send an explicit unknown account: %+v, %v", runs.request, err)
	}
	tracker := &webScheduleTracker{runs: runs, hypotheses: store}
	for range 2 {
		done, failed, _, err := tracker.RefTerminal(ctx, task, followup.ID)
		if err != nil || !done || failed {
			t.Fatalf("completed schedule failed follow-up: done=%v failed=%v error=%v", done, failed, err)
		}
	}
	records, err := store.Observations(source.ID, item.ID)
	if err != nil || len(records) != 1 || records[0].Status != "SUPPORTED" {
		t.Fatalf("unexpected observations: %+v, %v", records, err)
	}
}
