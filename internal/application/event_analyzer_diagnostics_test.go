package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	agentevent "Aegis/internal/agent/event"
	capevent "Aegis/internal/capability/event"
)

// Exercise the real Analyzer validators, not invented error strings. No model
// or external data source is called by these deterministic fault injections.
type diagnosticExtractor struct{ mode string }

func (e diagnosticExtractor) Extract(context.Context, capevent.ResearchTask, []capevent.CanonicalDocument) (agentevent.FactExtractionResult, error) {
	quote := "公司签订合同。"
	if e.mode == "grounding" {
		quote = "PRIVATE_UNSUPPORTED_QUOTE"
	}
	confidence := 0.8
	if e.mode == "confidence" {
		confidence = 2
	}
	return agentevent.FactExtractionResult{Confidence: confidence, Facts: []agentevent.ExtractedFact{{
		Type: agentevent.EventContract, Headline: "签订合同", Summary: "公司签订合同。", Confidence: 0.8,
		EvidenceQuotes: []agentevent.EvidenceQuote{{DocumentID: "doc", Quote: quote}},
	}}}, nil
}

type diagnosticClusterer struct{ invalid bool }

func (c diagnosticClusterer) Cluster(_ context.Context, _ capevent.ResearchTask, facts []agentevent.Fact) (agentevent.EventClusterResult, error) {
	id := facts[0].ID
	if c.invalid {
		id = "PRIVATE_UNKNOWN_FACT"
	}
	return agentevent.EventClusterResult{Clusters: []agentevent.EventCluster{{FactIDs: []string{id}}}}, nil
}

type diagnosticImpact struct{}

func (diagnosticImpact) AnalyzeImpactMechanisms(context.Context, capevent.ResearchTask, []agentevent.Fact, []agentevent.ClaimContradiction, []agentevent.FactRelation) (agentevent.ImpactMechanismDetectionResult, error) {
	return agentevent.ImpactMechanismDetectionResult{}, nil
}

type diagnosticMateriality struct{}

func (diagnosticMateriality) AnalyzeMateriality(context.Context, capevent.ResearchTask, agentevent.MaterialityInput) (agentevent.Materiality, error) {
	return agentevent.Materiality{}, nil // Missing required level must remain rejected.
}

func TestAnalyzerValidationFailuresPersistSpecificCategory(t *testing.T) {
	for _, tc := range []struct{ mode, category string }{
		{"confidence", "EVENT_FACT_EXTRACTION"},
		{"grounding", "EVENT_FACT_EXTRACTION"},
		{"cluster", "EVENT_FACT_CLUSTERING"},
		{"materiality", "EVENT_IMPACT_OR_MATERIALITY"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
			analyzer := agentevent.Analyzer{Extractor: diagnosticExtractor{mode: tc.mode}, Clusterer: diagnosticClusterer{invalid: tc.mode == "cluster"}, Impact: diagnosticImpact{}, Materiality: diagnosticMateriality{}}
			result, err := analyzer.Analyze(context.Background(), agentevent.AnalyzeInput{
				Task: capevent.ResearchTask{Symbol: "600519.SH", AsOf: now}, ResearchStatus: capevent.TaskSatisfied,
				Documents: []capevent.CanonicalDocument{{ID: "doc", Content: "公司签订合同。", Source: capevent.DocumentSource{Publisher: "issuer", IsPrimary: true}}},
			})
			if err == nil || result != nil {
				t.Fatal("invalid evidence or model result was accepted")
			}
			wrapped := fmt.Errorf("event research: analyze researched event evidence: %w", err)
			if got := researchErrorCategory(wrapped); got != tc.category {
				t.Fatalf("actual Analyzer error %v classified %q, want %q", err, got, tc.category)
			}
			assertEventFailureSurvivesRestart(t, wrapped, tc.category)
		})
	}
}

func TestAllEventAnalysisCategoriesSurviveRunRestart(t *testing.T) {
	for _, tc := range []struct{ message, category string }{
		{"extract event facts: invalid", "EVENT_FACT_EXTRACTION"},
		{"cluster event facts: invalid", "EVENT_FACT_CLUSTERING"},
		{"detect event x contradictions: invalid", "EVENT_EVENT_RELATION"},
		{"analyze event x materiality: invalid", "EVENT_IMPACT_OR_MATERIALITY"},
		{"normalize event x fact evolution: invalid", "EVENT_ANALYSIS_NORMALIZATION"},
		{"synthesize event analyst conclusion: invalid", "EVENT_ANALYST_SYNTHESIS"},
		{"normalize event analyst synthesis: invalid", "EVENT_ANALYST_SYNTHESIS_VALIDATION"},
		{"validate event context: invalid", "EVENT_ANALYST_CONTEXT_VALIDATION"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			assertEventFailureSurvivesRestart(t, errors.New("event research: analyze researched event evidence: "+tc.message+" PRIVATE_PROVIDER_PAYLOAD"), tc.category)
		})
	}
}

func assertEventFailureSurvivesRestart(t *testing.T, failure error, category string) {
	t.Helper()
	store := newStore(t)
	executor := &controlledRunExecutor{err: failure}
	manager := newMgr(t, executor, store, 1, 1)
	submitted, err := manager.Submit(context.Background(), runFixture("600519.SH"))
	if err != nil {
		t.Fatal(err)
	}
	failed := awaitRun(t, manager, submitted.ID, RunFailed)
	manager.Close()
	reopened, err := NewFileRunStore(store.dir)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := reopened.Get(context.Background(), submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.FailureCategory != category || saved.FailureCategory != category || saved.State != RunFailed || saved.FinishedAt == nil || saved.FailureCode != FailureResearch {
		t.Fatalf("failed Run lost its diagnostic or terminal state: %+v", saved)
	}
	if executor.count() != 1 {
		t.Fatal("failure retried execution")
	}
	raw, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatal("provider payload persisted")
	}
}
