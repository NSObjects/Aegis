package research

import (
	"Aegis/internal/research/evidenceledger"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/semanticreview"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/risk"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func syntheticArchiveInput(t *testing.T, linked bool) ArchivedResearchInput {
	t.Helper()
	w, _, managerStub, _, _ := reviewWorkflowFixture(t, reviewContextFixture(risk.AssessmentHighRisk))
	if linked {
		managerStub.plan.Opportunities = []manager.Insight{{
			Claim: "fixture-only technical possibility",
			References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}},
			ArgumentIDs: []string{"bull-1"},
		}}
	}
	artifact := reviewArtifactFixture()
	review, err := w.Run(context.Background(), artifact, reviewBookFixture())
	if err != nil { t.Fatal(err) }
	raw, err := json.Marshal(&FullPortfolioResult{ResearchArtifact: artifact, PortfolioReview: review})
	if err != nil { t.Fatal(err) }
	sourceBytes := []byte("fictional archival material, not a market document")
	evidenceHash := sha256.Sum256(sourceBytes)
	researchHash := sha256.Sum256(raw)
	return ArchivedResearchInput{
		SampleID: "fixture-sample-1", ResearchJSON: raw,
		ResearchSHA256: hex.EncodeToString(researchHash[:]),
		EvidenceBytes: sourceBytes,
		Source: semanticreview.CurationSource{
			EvidenceRef: "fictional-archive/research-source-1",
			EvidenceSHA256: hex.EncodeToString(evidenceHash[:]),
			EvidenceAsOf: "2026-09-23T13:00:00+08:00",
			CapturedAt: "2026-09-23T14:00:00+08:00",
			RequestedAsOf: artifact.Event.RequestedAsOf,
		},
	}
}

func changeArchivedResult(t *testing.T, input *ArchivedResearchInput, edit func(*FullPortfolioResult)) {
	t.Helper()
	var result FullPortfolioResult
	if err := json.Unmarshal(input.ResearchJSON, &result); err != nil { t.Fatal(err) }
	edit(&result)
	updated, err := json.Marshal(&result)
	if err != nil { t.Fatal(err) }
	input.ResearchJSON = updated
	hash := sha256.Sum256(updated)
	input.ResearchSHA256 = hex.EncodeToString(hash[:])
}

func TestCaptureArchivedResearchValidatesRealOutputShapeAndKeepsGoldEmpty(t *testing.T) {
	input := syntheticArchiveInput(t, true)
	got, err := CaptureArchivedResearch(input)
	if err != nil { t.Fatal(err) }
	if got.SampleID != input.SampleID || got.ResearchSHA256 != input.ResearchSHA256 ||
		got.Source != input.Source || got.Tasks.Symbol != "600519.SH" ||
		len(got.Tasks.Items) != 1 || got.Tasks.Items[0].ArgumentID != "bull-1" {
		t.Fatalf("candidate changed provenance or invented tasks: %+v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(encoded), "gold") || strings.Contains(string(encoded), "annotations") ||
		strings.Contains(string(encoded), "expected_verdict") {
		t.Fatalf("capture must not auto-generate human gold: %s", encoded)
	}
	// The freeze API does not accept an unreviewed capture as human labels.
	curated := &semanticreview.CurationBatch{Version: "fictional-v1", Samples: []semanticreview.CurationSample{{
		ID: got.SampleID, Tasks: got.Tasks, Source: got.Source,
		Split: semanticreview.SplitDevelopment, LeakageGroup: "same-research-fixture",
	}}}
	if frozen, err := semanticreview.FreezeCuration(curated); frozen != nil || err == nil {
		t.Fatalf("unlabeled captured output was frozen as human reviewed: %+v %v", frozen, err)
	}
	// No alias to the original source evidence buffer is retained.
	input.EvidenceBytes[0] = 'X'
	if got.Source.EvidenceSHA256 == "" || got.Tasks.Items[0].ArgumentID != "bull-1" {
		t.Fatal("mutable caller evidence unexpectedly changed candidate")
	}
}

func TestCaptureArchivedResearchRejectsUntrustedOutputAndEvidence(t *testing.T) {
	cases := []struct {
		name string
		change func(*testing.T, *ArchivedResearchInput)
		want string
	}{
		{"tampered research payload", func(_ *testing.T, in *ArchivedResearchInput) { in.ResearchJSON[0] = '!' }, "research archive sha256 mismatch"},
		{"incorrect research digest", func(_ *testing.T, in *ArchivedResearchInput) { in.ResearchSHA256 = strings.Repeat("0", 64) }, "research archive sha256 mismatch"},
		{"tampered evidence payload", func(_ *testing.T, in *ArchivedResearchInput) { in.EvidenceBytes[0] = '!' }, "source evidence sha256 mismatch"},
		{"invalid evidence digest", func(_ *testing.T, in *ArchivedResearchInput) { in.Source.EvidenceSHA256 = "bad-hash" }, "evidence_sha256"},
		{"missing evidence archive", func(_ *testing.T, in *ArchivedResearchInput) { in.EvidenceBytes = nil }, "source evidence archive"},
		{"source captured after cutoff", func(_ *testing.T, in *ArchivedResearchInput) { in.Source.CapturedAt = "2026-09-23T16:00:00+08:00" }, "point-in-time"},
		{"source evidence observed after capture", func(_ *testing.T, in *ArchivedResearchInput) { in.Source.EvidenceAsOf = "2026-09-23T14:30:00+08:00" }, "point-in-time"},
		{"archived decision cutoff does not match manifest", func(_ *testing.T, in *ArchivedResearchInput) { in.Source.RequestedAsOf = "2026-09-23T17:00:00+08:00" }, "cutoff does not match"},
		{"malformed archive with correct digest", func(_ *testing.T, in *ArchivedResearchInput) {
			in.ResearchJSON = []byte("not-json")
			h := sha256.Sum256(in.ResearchJSON)
			in.ResearchSHA256 = hex.EncodeToString(h[:])
		}, "decode archived"},
		{"extra top-level JSON with correct digest", func(_ *testing.T, in *ArchivedResearchInput) {
			in.ResearchJSON = append(append([]byte(nil), in.ResearchJSON...), []byte("{}")...)
			h := sha256.Sum256(in.ResearchJSON)
			in.ResearchSHA256 = hex.EncodeToString(h[:])
		}, "multiple JSON values"},
		{"missing ledger", func(t *testing.T, in *ArchivedResearchInput) {
			changeArchivedResult(t, in, func(result *FullPortfolioResult) { result.PortfolioReview.EvidenceLedger = nil })
		}, "missing required"},
		{"forged ledger classification", func(t *testing.T, in *ArchivedResearchInput) {
			changeArchivedResult(t, in, func(result *FullPortfolioResult) {
				result.PortfolioReview.EvidenceLedger.Arguments[0].Disposition = evidenceledger.DispositionUnresolved
			})
		}, "ledger differs"},
		{"unknown manager argument", func(t *testing.T, in *ArchivedResearchInput) {
			changeArchivedResult(t, in, func(result *FullPortfolioResult) {
				result.PortfolioReview.ResearchPlan.Opportunities[0].ArgumentIDs[0] = "ghost"
			})
		}, "does not exist in debate"},
		{"event capture after cutoff", func(t *testing.T, in *ArchivedResearchInput) {
			changeArchivedResult(t, in, func(result *FullPortfolioResult) {
				result.ResearchArtifact.Event.EvidenceAsOf = "2026-09-23T16:00:00+08:00"
			})
		}, "event evidence_as_of"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := syntheticArchiveInput(t, true)
			tc.change(t, &in)
			got, err := CaptureArchivedResearch(in)
			if err == nil || got != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("untrusted archive accepted: candidate=%+v err=%v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestCaptureArchivedResearchRejectsUnlinkedOutputs(t *testing.T) {
	in := syntheticArchiveInput(t, false)
	got, err := CaptureArchivedResearch(in)
	if err == nil || got != nil || !strings.Contains(err.Error(), "no explicit") {
		t.Fatalf("empty semantic worklist accepted: candidate=%+v err=%v", got, err)
	}
}

func TestCaptureArchivedResearchSnapshotMetadataDoesNotClaimArchivedOutputTime(t *testing.T) {
	// Source.CapturedAt is evidence capture, not the time the final report
	// completed. A final research output may be stored after the PIT cutoff.
	in := syntheticArchiveInput(t, true)
	inputCutoff := in.Source.RequestedAsOf
	got, err := CaptureArchivedResearch(in)
	if err != nil { t.Fatal(err) }
	if got.Source.RequestedAsOf != inputCutoff || got.ResearchSHA256 != in.ResearchSHA256 {
		t.Fatalf("source archive time semantics changed: %+v", got)
	}
}
