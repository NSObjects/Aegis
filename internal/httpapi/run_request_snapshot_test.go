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
		Events: []application.RunEvent{{Seq: 1, Stage: "Run", Status: "PENDING", At: cutoff}},
	}
	safe := publicRun(rec)
	b, err := json.Marshal(safe)
	if err != nil { t.Fatal(err) }
	for _, forbidden := range []string{"provider_key", "DO_NOT_EXPOSE", "HIDDEN_DIGEST", "HIDDEN_FINGERPRINT", "\"events\""} {
		if strings.Contains(string(b), forbidden) { t.Fatalf("unsafe public Run: %s", b) }
	}
	if safe.RequestSnapshot == nil ||
		safe.RequestSnapshot.SchemaVersion != 1 ||
		safe.RequestSnapshot.PortfolioState != "NOT_PROVIDED" ||
		safe.RequestSnapshot.EventWindowStart == nil {
		t.Fatalf("public Run lost explicit safe provenance: %#v", safe.RequestSnapshot)
	}
}
