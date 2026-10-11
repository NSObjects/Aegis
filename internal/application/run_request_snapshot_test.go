package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"Aegis/internal/portfolio"
)

func TestRunRequestSnapshotSurvivesCompletedRestartWithoutSensitiveInput(t *testing.T) {
	store := newStore(t)
	m := newMgr(t, &controlledRunExecutor{}, store, 1, 2)
	cutoff := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	input, err := NewLiveResearchInput("600519.SH", "PRIVATE_QUERY_DO_NOT_PERSIST", cutoff, 7*24*time.Hour)
	if err != nil { t.Fatal(err) }
	r, err := m.Submit(context.Background(), ResearchRequest{Mode: ModeDecision, Input: input})
	if err != nil { t.Fatal(err) }
	if r.RequestSnapshot == nil || r.RequestSnapshot.SchemaVersion != 1 {
		t.Fatalf("missing submit request snapshot: %#v", r)
	}
	if !r.RequestSnapshot.MarketQueryProvided ||
		r.RequestSnapshot.EventWindowStart == nil ||
		r.RequestSnapshot.EventWindowEnd == nil ||
		!r.RequestSnapshot.EventWindowStart.Equal(cutoff.Add(-7*24*time.Hour)) ||
		!r.RequestSnapshot.EventWindowEnd.Equal(cutoff) ||
		!r.RequestSnapshot.EventPITSafe ||
		r.RequestSnapshot.MinDocuments != 1 ||
		r.RequestSnapshot.MinIndependentSources != 1 ||
		len(r.RequestSnapshot.PreferredSourceTypes) == 0 {
		t.Fatalf("incorrect real submitted PIT constraints: %#v", r.RequestSnapshot)
	}
	// The submit response must not share any mutable storage with the
	// executor's pending RunRecord or its persisted snapshot.
	r.RequestSnapshot.PreferredSourceTypes[0] = "MUTATED"
	r.RequestSnapshot.EventWindowStart = nil
	r.Events[0].Stage = "MUTATED"
	done := awaitRun(t, m, r.ID, RunCompleted)
	if done.RequestSnapshot == nil || done.RequestSnapshot.EventWindowStart == nil ||
		done.RequestSnapshot.PreferredSourceTypes[0] == "MUTATED" ||
		done.Events[0].Stage == "MUTATED" {
		t.Fatalf("caller changed durable provenance: %#v", done)
	}
	serialized, err := json.Marshal(done)
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(serialized), "PRIVATE_QUERY_DO_NOT_PERSIST") ||
		strings.Contains(string(serialized), "MUTATED") {
		t.Fatalf("run record leaked raw query or caller mutation: %s", serialized)
	}
	m.Close()
	restarted := newMgr(t, &controlledRunExecutor{}, store, 1, 2)
	restored, err := restarted.Get(context.Background(), r.ID)
	if err != nil { t.Fatal(err) }
	if restored.RequestSnapshot == nil ||
		restored.RequestSnapshot.SchemaVersion != 1 ||
		!restored.RequestSnapshot.EventWindowEnd.Equal(cutoff) ||
		restored.State != RunCompleted {
		t.Fatalf("lost durable request snapshot after restart: %#v", restored)
	}
}

func TestRequestSnapshotStoresOnlyExplicitAccountState(t *testing.T) {
	store := newStore(t)
	m := newMgr(t, &controlledRunExecutor{}, store, 1, 2)
	cutoff := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	input, err := NewLiveResearchInput("600519.SH", "", cutoff, 48*time.Hour)
	if err != nil { t.Fatal(err) }
	rec, err := m.Submit(context.Background(), ResearchRequest{
		Mode: ModePortfolio,
		Input: input,
		Portfolio: &portfolio.Context{State: portfolio.StateNotProvided},
	})
	if err != nil { t.Fatal(err) }
	if rec.RequestSnapshot == nil || rec.RequestSnapshot.PortfolioState != "NOT_PROVIDED" ||
		rec.RequestSnapshot.MarketQueryProvided {
		t.Fatalf("account unknown was not retained: %#v", rec.RequestSnapshot)
	}
	// Fake executor is a Decision response and must fail closed. Its failed
	// Run still retains its request cutoff and explicit unknown-account state.
	failed := awaitRun(t, m, rec.ID, RunFailed)
	if failed.Artifact != nil || failed.RequestSnapshot == nil ||
		failed.RequestSnapshot.PortfolioState != "NOT_PROVIDED" {
		t.Fatalf("failed study lost audit metadata or invented an artifact: %#v", failed)
	}
}

func TestRequestSnapshotValidatesCorruptionButReadsLegacyRecords(t *testing.T) {
	store := newStore(t)
	m := newMgr(t, &controlledRunExecutor{}, store, 1, 2)
	rec, err := m.Submit(context.Background(), runFixture("600519.SH"))
	if err != nil { t.Fatal(err) }
	done := awaitRun(t, m, rec.ID, RunCompleted)
	if done.RequestSnapshot == nil { t.Fatal("new runs require a snapshot") }
	original := done.RequestSnapshot
	bad := []RunRequestSnapshot{
		{SchemaVersion: 2},
		{SchemaVersion: 1, EventWindowStart: &done.RequestedAsOf},
		{SchemaVersion: 1, EventWindowStart: &done.RequestedAsOf, EventWindowEnd: &done.RequestedAsOf},
		{SchemaVersion: 1, MinDocuments: -1},
		{SchemaVersion: 1, PortfolioState: "POSITIONS"},
	}
	for _, meta := range bad {
		invalid := done
		invalid.RequestSnapshot = &meta
		if err := store.Save(context.Background(), invalid); err == nil {
			t.Fatalf("accepted malformed request snapshot: %#v", meta)
		}
	}
	done.RequestSnapshot = original
	// Pre-provenance records remain readable, without pretending to know
	// model versions, source types or the user's original research settings.
	done.RequestSnapshot = nil
	if err := store.Save(context.Background(), done); err != nil { t.Fatal(err) }
	legacy, err := store.Get(context.Background(), rec.ID)
	if err != nil || legacy.RequestSnapshot != nil {
		t.Fatalf("lost backwards compatibility: %#v %v", legacy, err)
	}
}
