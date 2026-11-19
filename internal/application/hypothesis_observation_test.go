package application

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	eventagent "Aegis/internal/agent/event"
	"Aegis/internal/capability/technical"
)

func TestHypothesisObservationUsesOnlyTargetDayAndPreservesHistory(t *testing.T) {
	asOf := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
	source := RunRecord{
		ID: strings.Repeat("a", 32), Symbol: "600519.SH", State: RunCompleted,
		RequestedAsOf: asOf, Artifact: json.RawMessage(`{"source":true}`),
	}
	dir := filepath.Join(t.TempDir(), "hypotheses")
	store, err := NewFileHypothesisStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	draft := HypothesisDraft{
		Statement: "目标日收盘价不低于 100", Invalidation: "低于阈值",
		VerificationRule: "查看未复权收盘价", ExpiresAt: asOf.Add(48 * time.Hour),
		CloseRule: &HypothesisCloseRule{TradeDate: "20261117", Operator: "GTE", Threshold: 100},
	}
	item, _, err := store.Add(source, draft, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Observe(source.ID, item.ID, source); !errors.Is(err, ErrObservationSource) {
		t.Fatalf("source run was accepted as a follow-up: %v", err)
	}
	base := source
	base.RequestedAsOf = asOf.Add(24 * time.Hour)
	base.HistoricalOHLCV = &technical.HistoricalObservation{
		Symbol: source.Symbol, PriceBasis: "RAW", EffectiveAsOf: "20261117",
		Bars: []technical.HistoricalBar{{TradeDate: "20261116", Close: 150}},
	}
	for _, tc := range []struct {
		id, status string
		close      float64
		hours      int
	}{
		{"b", "INCONCLUSIVE", 0, 24},
		{"c", "EXPIRED", 0, 72},
		{"d", "REFUTED", 99, 24},
		{"e", "SUPPORTED", 100, 72},
		{"f", "INCONCLUSIVE", 101, 22},
	} {
		run := base
		run.ID = strings.Repeat(tc.id, 32)
		run.RequestedAsOf = asOf.Add(time.Duration(tc.hours) * time.Hour)
		if tc.close > 0 {
			series := *base.HistoricalOHLCV
			series.Bars = []technical.HistoricalBar{{TradeDate: "20261117", Close: tc.close}}
			run.HistoricalOHLCV = &series
		}
		observation, created, err := store.Observe(source.ID, item.ID, run)
		if err != nil || !created || observation.Status != tc.status {
			t.Fatalf("want %s, got %+v, %v", tc.status, observation, err)
		}
		if _, created, err := store.Observe(source.ID, item.ID, run); err != nil || created {
			t.Fatalf("observation was duplicated on retry: created=%v err=%v", created, err)
		}
	}
	reopened, err := NewFileHypothesisStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	records, err := reopened.Observations(source.ID, item.ID)
	if err != nil || len(records) != 5 {
		t.Fatalf("expected five persisted observations, got %d: %v", len(records), err)
	}
	for _, sample := range []struct{ digit, outcome string }{{"d", "REFUTED"}, {"e", "SUPPORTED"}} {
		laterID := strings.Repeat(sample.digit, 32)
		entry, created, err := reopened.AddMemory(MemorySource{
			SourceRunID: source.ID, HypothesisID: item.ID, ObservationRunID: laterID,
		}, asOf.Add(96*time.Hour))
		if err != nil || !created || entry.Outcome != sample.outcome {
			t.Fatalf("memory must preserve the observed outcome: %+v %v", entry, err)
		}
		results, err := reopened.SearchMemory(MemoryFilter{Outcome: sample.outcome})
		if err != nil || results.Total != 1 || results.Memories[0].ObservationRunID != laterID {
			t.Fatalf("memory outcome search mismatch: %+v %v", results, err)
		}
	}
	items, err := reopened.List(source.ID)
	if err != nil || len(items) != 1 || items[0].ID != item.ID || items[0].Statement != item.Statement {
		t.Fatalf("original hypothesis changed: %+v, %v", items, err)
	}
}

func TestHypothesisDocumentRequiresPrimaryPointInTimeEvidence(t *testing.T) {
	start := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
	deadline := start.Add(24 * time.Hour)
	publication, availability := start.Add(time.Hour), start.Add(2*time.Hour)
	item := ResearchHypothesis{
		AsOf: start,
		HypothesisDraft: HypothesisDraft{
			ExpiresAt:    deadline,
			DocumentRule: &HypothesisDocumentRule{DocumentID: "announcement", OnMatch: "REFUTED"},
		},
	}
	run := RunRecord{
		RequestedAsOf: deadline.Add(time.Hour),
		EvidenceIndex: &RunEvidenceIndex{
			DocumentIDs: []string{"announcement"},
			Documents: []eventagent.CitedDocumentSource{{
				DocumentID: "announcement", IsPrimary: true, PublishedAt: &publication, AvailableAt: &availability,
			}},
		},
	}
	if got := evaluateHypothesis(item, run); got.Status != "REFUTED" || got.DocumentID != "announcement" {
		t.Fatalf("document should refute the hypothesis: %+v", got)
	}
	for _, tc := range []struct {
		label  string
		adjust func(*eventagent.CitedDocumentSource)
	}{
		{"missing availability", func(doc *eventagent.CitedDocumentSource) { doc.AvailableAt = nil }},
		{"late availability", func(doc *eventagent.CitedDocumentSource) { doc.AvailableAt = &run.RequestedAsOf }},
		{"old publication", func(doc *eventagent.CitedDocumentSource) { doc.PublishedAt = &start }},
		{"secondary source", func(doc *eventagent.CitedDocumentSource) { doc.IsPrimary = false }},
	} {
		t.Run(tc.label, func(t *testing.T) {
			index := *run.EvidenceIndex
			doc := index.Documents[0]
			tc.adjust(&doc)
			index.Documents = []eventagent.CitedDocumentSource{doc}
			later := run
			later.EvidenceIndex = &index
			if got := evaluateHypothesis(item, later); got.Status != "EXPIRED" || got.DocumentID != "" {
				t.Fatalf("ineligible evidence produced a verdict: %+v", got)
			}
		})
	}
}
