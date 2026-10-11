package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"Aegis/internal/application"
)

func TestPublicRunKeepsSafeInputProvenanceWithoutPrivatePayloads(t *testing.T) {
	cutoff := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	start := cutoff.Add(-7 * 24 * time.Hour)
	rec := application.RunRecord{
		ID: strings.Repeat("a", 32),
		Mode: application.ModePortfolio,
		Symbol: "600519.SH", RequestedAsOf: cutoff, CreatedAt: cutoff,
		State: application.RunCompleted, Artifact: json.RawMessage(`{"provider_key":"DO_NOT_EXPOSE"}`),
		IdempotencyDigest: "HIDDEN_DIGEST", RequestFingerprint: "HIDDEN_FINGERPRINT",
		RequestSnapshot: &application.RunRequestSnapshot{
			SchemaVersion: 1, MarketQueryProvided: true,
			EventWindowStart: &start, EventWindowEnd: &cutoff,
			EventPITSafe: true, PreferredSourceTypes: []string{"OFFICIAL"},
			MinDocuments: 1, MinIndependentSources: 1,
			PortfolioState: "NOT_PROVIDED",
		},
		ModelProvenance: &application.RunModelProvenance{
			SchemaVersion: 1, ConfigFingerprint: strings.Repeat("a", 64),
			Bindings: []application.ModelRoleBinding{{Role: "market", Profile: "default", Provider: "deepseek", Model: "deepseek-chat"}},
		},
		EvidenceIndex: &application.RunEvidenceIndex{
			SchemaVersion: 1, Status: "NORMAL", RequestedAsOf: cutoff.Format(time.RFC3339),
			Events: []application.RunEvidenceEvent{{
				ID: "event-abc", SourceCount: 2, DocumentIDs: []string{"doc-1"},
			}}, DocumentIDs: []string{"doc-1"},
		},
		Events: []application.RunEvent{{Seq: 1, Stage: "Run", Status: "PENDING", At: cutoff}},
	}
	safe := publicRun(rec)
	b, err := json.Marshal(safe)
	if err != nil { t.Fatal(err) }
	for _, forbidden := range []string{"provider_key", "DO_NOT_EXPOSE", "HIDDEN_DIGEST", "HIDDEN_FINGERPRINT"} {
		if strings.Contains(string(b), forbidden) { t.Fatalf("unsafe public Run: %s", b) }
	}
	if len(safe.Events) != 0 || len(safe.Artifact) != 0 {
		t.Fatalf("public Run exposed internal stage events or raw Artifact: %s", b)
	}
	if safe.RequestSnapshot == nil ||
		safe.RequestSnapshot.SchemaVersion != 1 ||
		safe.RequestSnapshot.PortfolioState != "NOT_PROVIDED" ||
		safe.RequestSnapshot.EventWindowStart == nil {
		t.Fatalf("public Run lost explicit safe provenance: %#v", safe.RequestSnapshot)
	}
	if safe.ModelProvenance == nil || safe.ModelProvenance.SchemaVersion != 1 ||
		safe.ModelProvenance.Bindings[0].Model != "deepseek-chat" ||
		!strings.Contains(string(b), "config_fingerprint") {
		t.Fatalf("public Run lost nonsecret actual model identity: %s", b)
	}
	if safe.EvidenceIndex == nil || safe.EvidenceIndex.Events[0].ID != "event-abc" ||
		!strings.Contains(string(b), "\"evidence_index\"") || strings.Contains(string(b), "https://") {
		t.Fatalf("public Run lost safe source identities or invented links: %s", b)
	}
}
