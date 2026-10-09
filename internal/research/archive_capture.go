package research

import (
	"Aegis/internal/research/evidenceledger"
	"Aegis/internal/research/semanticreview"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
)

const maxArchivedResearchBytes = 16 << 20
const maxArchivedEvidenceBytes = 16 << 20

// ArchivedResearchInput binds the exact saved FullPortfolioResult bytes to
// separately archived, point-in-time source-evidence bytes. Neither archive
// can be reconstructed from an LLM summary or invented by the capture routine.
// CapturedAt describes the EVIDENCE archive, not when the final research report
// was written; that report may be saved after the decision cutoff.
type ArchivedResearchInput struct {
	SampleID             string
	ResearchJSON         []byte
	ResearchSHA256       string
	EvidenceBytes        []byte
	Source               semanticreview.CurationSource
}

// ResearchSampleCandidate is UNLABELED. It cannot be used as human-reviewed
// gold and is not an EvaluationDataset. Source authenticity, human identities,
// evidence-to-claim correspondence and archive custody require external audit.
type ResearchSampleCandidate struct {
	SampleID       string                        `json:"sample_id"`
	ResearchSHA256 string                        `json:"research_sha256"`
	Source         semanticreview.CurationSource `json:"source"`
	Tasks          *semanticreview.Tasks         `json:"tasks"`
}

// CaptureArchivedResearch checks exact archived bytes, upstream typed research
// contracts, ledger consistency and PIT metadata. It does not invoke providers,
// create labels, assign holdout splits, or assert that any source is real.
// It intentionally rejects reports without explicit reviewable links.
func CaptureArchivedResearch(input ArchivedResearchInput) (*ResearchSampleCandidate, error) {
	if strings.TrimSpace(input.SampleID) == "" || input.SampleID != strings.TrimSpace(input.SampleID) {
		return nil, errors.New("canonical sample_id is required")
	}
	if len(input.ResearchJSON) == 0 || len(input.ResearchJSON) > maxArchivedResearchBytes {
		return nil, errors.New("research archive must be nonempty and within size limit")
	}
	if len(input.EvidenceBytes) == 0 || len(input.EvidenceBytes) > maxArchivedEvidenceBytes {
		return nil, errors.New("source evidence archive must be nonempty and within size limit")
	}
	if err := semanticreview.ValidateCurationSource(input.Source); err != nil {
		return nil, fmt.Errorf("invalid archived source: %w", err)
	}
	researchHash := sha256.Sum256(input.ResearchJSON)
	researchSHA256 := hex.EncodeToString(researchHash[:])
	if input.ResearchSHA256 != researchSHA256 {
		return nil, errors.New("research archive sha256 mismatch")
	}
	evidenceHash := sha256.Sum256(input.EvidenceBytes)
	evidenceSHA256 := hex.EncodeToString(evidenceHash[:])
	if input.Source.EvidenceSHA256 != evidenceSHA256 {
		return nil, errors.New("source evidence sha256 mismatch")
	}

	var archived FullPortfolioResult
	decoder := json.NewDecoder(bytes.NewReader(input.ResearchJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&archived); err != nil {
		return nil, fmt.Errorf("decode archived FullPortfolioResult: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("research archive contains multiple JSON values")
		}
		return nil, fmt.Errorf("research archive trailing JSON: %w", err)
	}
	if archived.ResearchArtifact == nil || archived.PortfolioReview == nil ||
		archived.PortfolioReview.Debate == nil || archived.PortfolioReview.ResearchPlan == nil ||
		archived.PortfolioReview.EvidenceLedger == nil ||
		archived.PortfolioReview.TradeIntent == nil ||
		archived.PortfolioReview.RiskContext == nil ||
		archived.PortfolioReview.PortfolioDecision == nil {
		return nil, errors.New("archived FullPortfolioResult is missing required research or review stages")
	}
	artifact, review := archived.ResearchArtifact, archived.PortfolioReview
	requestedAt, _ := time.Parse(time.RFC3339, input.Source.RequestedAsOf) // validated above
	if artifact.Event == nil {
		return nil, errors.New("archived research event context is missing")
	}
	eventRequestedAt, err := time.Parse(time.RFC3339, artifact.Event.RequestedAsOf)
	if err != nil || !eventRequestedAt.Equal(requestedAt) {
		return nil, errors.New("archived research cutoff does not match source requested_as_of")
	}
	if strings.TrimSpace(artifact.Event.EvidenceAsOf) != "" {
		eventEvidenceAt, err := time.Parse(time.RFC3339, artifact.Event.EvidenceAsOf)
		if err != nil || eventEvidenceAt.After(requestedAt) {
			return nil, errors.New("archived event evidence_as_of violates research cutoff")
		}
	}

	projected, err := evidenceledger.Build(artifact, review.Debate, review.ResearchPlan)
	if err != nil {
		return nil, fmt.Errorf("invalid archived research provenance: %w", err)
	}
	if !reflect.DeepEqual(projected, review.EvidenceLedger) {
		return nil, errors.New("archived evidence ledger differs from validated research sources")
	}
	tasks, err := semanticreview.BuildTasks(artifact, review.Debate, review.ResearchPlan)
	if err != nil {
		return nil, fmt.Errorf("derive archived review tasks: %w", err)
	}
	if len(tasks.Items) == 0 {
		return nil, errors.New("archived research has no explicit semantic review task links")
	}
	return &ResearchSampleCandidate{
		SampleID: input.SampleID, ResearchSHA256: researchSHA256,
		Source: input.Source, Tasks: tasks,
	}, nil
}
