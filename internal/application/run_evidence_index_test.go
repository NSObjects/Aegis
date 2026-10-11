package application

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	eventagent "Aegis/internal/agent/event"
	"Aegis/internal/research"
	"Aegis/internal/portfolio"
	"Aegis/internal/research/synthesis"
)

type indexedPortfolioExecutor struct {
	status eventagent.Status
	fail bool
}

func (x *indexedPortfolioExecutor) Run(_ context.Context, req ResearchRequest) (*ResearchResponse, error) {
	if x.fail { return nil, errors.New("provider failure") }
	eventCtx := &eventagent.Context{
		Symbol: req.Input.Symbol,
		RequestedAsOf: req.Input.RequestedAsOf.UTC().Format(time.RFC3339),
		EvidenceAsOf: req.Input.RequestedAsOf.Add(-time.Minute).UTC().Format(time.RFC3339),
		Status: x.status,
	}
	if x.status == eventagent.StatusNormal {
		eventCtx.Events = []eventagent.Event{{
			ID: "evt-alpha", OccurredAt: req.Input.RequestedAsOf.Add(-time.Hour).UTC().Format(time.RFC3339),
			EvidenceDocumentIDs: []string{"doc-b", "doc-a"}, SourceCount: 2,
			Facts: []eventagent.Fact{{
				EvidenceDocumentIDs: []string{"doc-a"},
				Groundings: []eventagent.EvidenceGrounding{{DocumentID: "doc-b"}},
			}},
		}}
	}
	return &ResearchResponse{Mode: ModePortfolio, Portfolio: &research.FullPortfolioResult{
		ResearchArtifact: &synthesis.Artifact{Symbol: req.Input.Symbol, Event: eventCtx},
	}}, nil
}

func portfolioIndexFixture() ResearchRequest {
	req := runFixture("600519.SH")
	req.Mode = ModePortfolio
	req.Portfolio = nil // replaced by test with explicit unknown account
	return req
}

func TestEvidenceIndexFromActualCompletedArtifactSurvivesRestart(t *testing.T) {
	store := newStore(t)
	m := newMgr(t, &indexedPortfolioExecutor{status: eventagent.StatusNormal}, store, 1, 2)
	req := portfolioIndexFixture()
	// This unit test directly indexes a synthetic typed result. It is not
	// claiming that any provider source actually returned live documents.
	req.Portfolio = &portfolio.Context{State: portfolio.StateNotProvided}
	submitted, err := m.Submit(context.Background(), req)
	if err != nil { t.Fatal(err) }
	if submitted.EvidenceIndex != nil { t.Fatal("PENDING Run claims completed evidence") }
	done := awaitRun(t, m, submitted.ID, RunCompleted)
	idx := done.EvidenceIndex
	if idx == nil || idx.SchemaVersion != 1 || idx.Status != "NORMAL" ||
		len(idx.Events) != 1 || idx.Events[0].ID != "evt-alpha" ||
		idx.Events[0].SourceCount != 2 ||
		!reflect.DeepEqual(idx.DocumentIDs, []string{"doc-a", "doc-b"}) ||
		!reflect.DeepEqual(idx.Events[0].DocumentIDs, []string{"doc-a", "doc-b"}) {
		t.Fatalf("missing actual typed artifact identities: %#v", idx)
	}
	raw, err := json.Marshal(idx)
	if err != nil { t.Fatal(err) }
	for _, forbidden := range []string{"https://", "api_key", "groundings", "quote", "source_url"} {
		if strings.Contains(string(raw), forbidden) { t.Fatalf("unsafe metadata on Run: %s", raw) }
	}
	m.Close()
	after := newMgr(t, &indexedPortfolioExecutor{status: eventagent.StatusNoEvent}, store, 1, 2)
	saved, err := after.Get(context.Background(), submitted.ID)
	if err != nil || !reflect.DeepEqual(saved.EvidenceIndex, idx) {
		t.Fatalf("restart lost exact typed source IDs: %#v %v", saved.EvidenceIndex, err)
	}
	if len(saved.Artifact) == 0 { t.Fatal("completed artifact missing after restart") }
}

func TestEvidenceIndexPreservesNoEventVsMissingAndFailed(t *testing.T) {
	store := newStore(t)
	m := newMgr(t, &indexedPortfolioExecutor{status: eventagent.StatusNoEvent}, store, 1, 3)
	req := portfolioIndexFixture()
	req.Portfolio = &portfolio.Context{State: portfolio.StateNotProvided}
	rec, err := m.Submit(context.Background(), req)
	if err != nil { t.Fatal(err) }
	done := awaitRun(t, m, rec.ID, RunCompleted)
	if done.EvidenceIndex == nil || done.EvidenceIndex.Status != "NO_EVENT" ||
		len(done.EvidenceIndex.Events) != 0 || len(done.EvidenceIndex.DocumentIDs) != 0 {
		t.Fatalf("NO_EVENT incorrectly treated as not recorded: %#v", done.EvidenceIndex)
	}
	m.Close()
	// An execution failure cannot emit completed-only provenance.
	failer := newMgr(t, &indexedPortfolioExecutor{fail: true}, store, 1, 2)
	req.Input.Symbol = "000001.SZ"
	bad, err := failer.Submit(context.Background(), req)
	if err != nil { t.Fatal(err) }
	failed := awaitRun(t, failer, bad.ID, RunFailed)
	if failed.EvidenceIndex != nil || len(failed.Artifact) > 0 {
		t.Fatalf("failed Run leaked indexed evidence: %#v", failed)
	}
}

func TestEvidenceIndexTamperingRejectedButLegacyStillReadable(t *testing.T) {
	store := newStore(t)
	m := newMgr(t, &indexedPortfolioExecutor{status: eventagent.StatusNormal}, store, 1, 2)
	req := portfolioIndexFixture()
	req.Portfolio = &portfolio.Context{State: portfolio.StateNotProvided}
	rec, err := m.Submit(context.Background(), req)
	if err != nil { t.Fatal(err) }
	done := awaitRun(t, m, rec.ID, RunCompleted)
	invalid := done
	copyIndex := *done.EvidenceIndex
	copyIndex.Events = append([]RunEvidenceEvent(nil), done.EvidenceIndex.Events...)
	copyIndex.DocumentIDs = []string{"invented-doc"}
	invalid.EvidenceIndex = &copyIndex
	if err := store.Save(context.Background(), invalid); err == nil { t.Fatal("tampered source index saved") }
	copyIndex = *done.EvidenceIndex
	copyIndex.Status = "NORMAL_WRONG"
	invalid.EvidenceIndex = &copyIndex
	if err := store.Save(context.Background(), invalid); err == nil { t.Fatal("incorrect status accepted") }
	invalid = done
	invalid.State = RunFailed
	invalid.FailureCode = FailureResearch
	invalid.Artifact = nil
	invalid.ArtifactVersion, invalid.ArtifactSHA256 = 0, ""
	if err := store.Save(context.Background(), invalid); err == nil { t.Fatal("failed Run accepted index") }
	// Old completed records without the new optional field remain readable
	// and are never backfilled with synthetic event IDs.
	legacy := done
	legacy.EvidenceIndex = nil
	if err := store.Save(context.Background(), legacy); err != nil { t.Fatal(err) }
	read, err := store.Get(context.Background(), done.ID)
	if err != nil || read.EvidenceIndex != nil { t.Fatalf("legacy record rejected: %#v %v",read,err) }
}

func TestEvidenceIndexRejectsAmbiguousOrMismatchedIdentity(t *testing.T) {
	req := portfolioIndexFixture()
	req.Portfolio = &portfolio.Context{State: portfolio.StateNotProvided}
	runner := &indexedPortfolioExecutor{status: eventagent.StatusNormal}
	res, err := runner.Run(context.Background(), req)
	if err != nil { t.Fatal(err) }
	a, err := json.Marshal(res.Output())
	if err != nil { t.Fatal(err) }
	if _, err := projectRunEvidenceIndex(a, ModePortfolio, "000001.SZ", req.Input.RequestedAsOf); err == nil {
		t.Fatal("accepted wrong run symbol")
	}
	if _, err := projectRunEvidenceIndex(a, ModePortfolio, req.Input.Symbol, req.Input.RequestedAsOf.Add(time.Hour)); err == nil {
		t.Fatal("accepted changed research cutoff")
	}
	if v, err := projectRunEvidenceIndex(a, ModeDecision, req.Input.Symbol, req.Input.RequestedAsOf); err != nil || v != nil {
		t.Fatal("Decision mode fabricated an Event index")
	}
	res.Portfolio.ResearchArtifact.Event.Events = append(res.Portfolio.ResearchArtifact.Event.Events, res.Portfolio.ResearchArtifact.Event.Events[0])
	duplicate, err := json.Marshal(res.Output())
	if err != nil { t.Fatal(err) }
	if _, err := projectRunEvidenceIndex(duplicate, ModePortfolio, req.Input.Symbol, req.Input.RequestedAsOf); err == nil {
		t.Fatal("duplicate event ID accepted as unique")
	}
}
