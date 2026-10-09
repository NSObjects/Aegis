// Package semanticreview provides offline research-quality contracts.
// Curation records are caller-supplied attestations, not proof of human identity
// or provenance. No curation operation invokes a model, data source or broker.
package semanticreview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

type DatasetSplit string

const (
	SplitDevelopment DatasetSplit = "DEVELOPMENT"
	SplitHoldout     DatasetSplit = "HOLDOUT"
)

// CurationSource points at an external point-in-time research snapshot.
// Hash shape and chronology are checked here; the actual bytes and identity
// of the source must be verified separately by the evidence custodian.
type CurationSource struct {
	EvidenceRef    string `json:"evidence_ref"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	EvidenceAsOf   string `json:"evidence_as_of"`
	CapturedAt     string `json:"captured_at"`
	RequestedAsOf  string `json:"requested_as_of"`
}

// AnnotationAttestation records each independent, blind human annotation.
// The two flags are unverified caller attestations, not platform enforcement.
type AnnotationAttestation struct {
	AnnotatorID    string      `json:"annotator_id"`
	BlindToModel   bool        `json:"blind_to_model"`
	BlindToPeer    bool        `json:"blind_to_peer"`
	Labels         []GoldLabel `json:"labels"`
}

// Adjudication is required for each disagreement, never automatically
// resolved by majority vote, reference overlap, or an LLM prediction.
type Adjudication struct {
	AdjudicatorID string    `json:"adjudicator_id"`
	Label         GoldLabel `json:"label"`
	Rationale     string    `json:"rationale"`
}

type CurationSample struct {
	ID            string                  `json:"id"`
	Split         DatasetSplit            `json:"split"`
	LeakageGroup  string                  `json:"leakage_group"`
	Source        CurationSource          `json:"source"`
	Tasks         *Tasks                  `json:"tasks"`
	Annotations   []AnnotationAttestation `json:"annotations"`
	Adjudications []Adjudication          `json:"adjudications,omitempty"`
}

type CurationBatch struct {
	Version string           `json:"version"`
	Samples []CurationSample `json:"samples"`
}

type FreezeManifest struct {
	Version            string `json:"version"`
	CorpusSHA256       string `json:"corpus_sha256"`
	DevelopmentSHA256  string `json:"development_sha256"`
	HoldoutSHA256      string `json:"holdout_sha256"`
	DevelopmentSamples int    `json:"development_samples"`
	HoldoutSamples     int    `json:"holdout_samples"`
	AdjudicatedItems   int    `json:"adjudicated_items"`
}

// FrozenCuration uses serialized byte snapshots: changes to caller-owned
// input objects after FreezeCuration cannot change the frozen datasets.
// A checksum detects accidental modification; it is NOT a signature, proof
// of human labeling, immutable remote storage, or holdout access control.
type FrozenCuration struct {
	Manifest        FreezeManifest   `json:"manifest"`
	CorpusJSON      json.RawMessage  `json:"corpus_json"`
	DevelopmentJSON json.RawMessage  `json:"development_json"`
	HoldoutJSON     json.RawMessage  `json:"holdout_json"`
}

func digestJSON(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func canonicalID(id string) bool {
	return id != "" && id == strings.TrimSpace(id)
}

func validVerdict(v Verdict) bool {
	switch v {
	case VerdictConsistent, VerdictContradictory, VerdictUncertain:
		return true
	default:
		return false
	}
}

func validateSource(src CurationSource) error {
	if !canonicalID(src.EvidenceRef) {
		return errors.New("evidence_ref is required and must be canonical")
	}
	decoded, err := hex.DecodeString(src.EvidenceSHA256)
	if err != nil || len(decoded) != sha256.Size || src.EvidenceSHA256 != strings.ToLower(src.EvidenceSHA256) {
		return errors.New("evidence_sha256 must be a lowercase 64-character SHA-256 hex digest")
	}
	evidenceAt, err := time.Parse(time.RFC3339, src.EvidenceAsOf)
	if err != nil {
		return fmt.Errorf("invalid evidence_as_of: %w", err)
	}
	capturedAt, err := time.Parse(time.RFC3339, src.CapturedAt)
	if err != nil {
		return fmt.Errorf("invalid captured_at: %w", err)
	}
	requestedAt, err := time.Parse(time.RFC3339, src.RequestedAsOf)
	if err != nil {
		return fmt.Errorf("invalid requested_as_of: %w", err)
	}
	// Strict default: the immutable source must have been captured on or
	// before the historical decision cutoff, not reconstructed afterward.
	if evidenceAt.After(capturedAt) || capturedAt.After(requestedAt) {
		return errors.New("source chronology violates point-in-time cutoff")
	}
	return nil
}

func validateAnnotation(labels []GoldLabel, tasks *Tasks) error {
	if len(labels) != len(tasks.Items) {
		return errors.New("annotation labels must cover every task in order")
	}
	for i, label := range labels {
		task := tasks.Items[i]
		if label.InsightIndex != task.InsightIndex || label.ArgumentID != task.ArgumentID || !validVerdict(label.ExpectedVerdict) {
			return fmt.Errorf("annotation label %d identity or verdict is invalid", i)
		}
	}
	return nil
}

// FreezeCuration rejects missing provenance metadata, leaked development-
// holdout groups, unblinded reviewers, incomplete labels and unadjudicated
// disagreements. It creates immutable *serialized* datasets using the existing
// offline EvaluationDataset contract. No model predictions are accepted here.
func FreezeCuration(input *CurationBatch) (*FrozenCuration, error) {
	if input == nil {
		return nil, errors.New("curation batch is nil")
	}
	if !canonicalID(input.Version) {
		return nil, errors.New("curation version is required and must be canonical")
	}
	if len(input.Samples) == 0 {
		return nil, errors.New("curation batch requires samples")
	}

	development := &EvaluationDataset{Version: input.Version + "-development", Provenance: ProvenanceHumanReviewed}
	holdout := &EvaluationDataset{Version: input.Version + "-holdout", Provenance: ProvenanceHumanReviewed}
	seenIDs := make(map[string]struct{}, len(input.Samples))
	groups := make(map[string]DatasetSplit)
	evidenceSplits := make(map[string]DatasetSplit)
	adjudicated := 0

	for index, sample := range input.Samples {
		if !canonicalID(sample.ID) || !canonicalID(sample.LeakageGroup) {
			return nil, fmt.Errorf("sample %d requires canonical id and leakage_group", index)
		}
		if _, duplicate := seenIDs[sample.ID]; duplicate {
			return nil, fmt.Errorf("duplicate sample_id %q", sample.ID)
		}
		seenIDs[sample.ID] = struct{}{}
		switch sample.Split {
		case SplitDevelopment, SplitHoldout:
		default:
			return nil, fmt.Errorf("sample %q has invalid split %q", sample.ID, sample.Split)
		}
		if existing, ok := groups[sample.LeakageGroup]; ok && existing != sample.Split {
			return nil, fmt.Errorf("leakage_group %q crosses development/holdout", sample.LeakageGroup)
		}
		groups[sample.LeakageGroup] = sample.Split
		if err := validateSource(sample.Source); err != nil {
			return nil, fmt.Errorf("sample %q source: %w", sample.ID, err)
		}
		if existing, ok := evidenceSplits[sample.Source.EvidenceSHA256]; ok && existing != sample.Split {
			return nil, fmt.Errorf("source snapshot %q crosses development/holdout", sample.Source.EvidenceSHA256)
		}
		evidenceSplits[sample.Source.EvidenceSHA256] = sample.Split
		if sample.Tasks == nil || len(sample.Tasks.Items) == 0 {
			return nil, fmt.Errorf("sample %q requires nonempty tasks", sample.ID)
		}
		if err := validateReviewerTasks(sample.Tasks, len(sample.Tasks.Items)); err != nil {
			return nil, fmt.Errorf("sample %q tasks: %w", sample.ID, err)
		}
		if len(sample.Annotations) != 2 {
			return nil, fmt.Errorf("sample %q requires exactly two independent annotations", sample.ID)
		}
		first, second := sample.Annotations[0], sample.Annotations[1]
		if !canonicalID(first.AnnotatorID) || !canonicalID(second.AnnotatorID) ||
			first.AnnotatorID == second.AnnotatorID {
			return nil, fmt.Errorf("sample %q requires two distinct canonical annotator_ids", sample.ID)
		}
		for _, annotation := range sample.Annotations {
			if !annotation.BlindToModel || !annotation.BlindToPeer {
				return nil, fmt.Errorf("sample %q requires model-blind and peer-blind annotation attestations", sample.ID)
			}
			if err := validateAnnotation(annotation.Labels, sample.Tasks); err != nil {
				return nil, fmt.Errorf("sample %q annotator %q: %w", sample.ID, annotation.AnnotatorID, err)
			}
		}

		consensus := make([]GoldLabel, len(sample.Tasks.Items))
		nextAdjudication := 0
		for taskIndex := range sample.Tasks.Items {
			left, right := first.Labels[taskIndex], second.Labels[taskIndex]
			if left.ExpectedVerdict == right.ExpectedVerdict {
				consensus[taskIndex] = left
				continue
			}
			if nextAdjudication >= len(sample.Adjudications) {
				return nil, fmt.Errorf("sample %q task %d disagreement lacks adjudication", sample.ID, taskIndex)
			}
			adjudication := sample.Adjudications[nextAdjudication]
			nextAdjudication++
			if !canonicalID(adjudication.AdjudicatorID) ||
				adjudication.AdjudicatorID == first.AnnotatorID || adjudication.AdjudicatorID == second.AnnotatorID ||
				!canonicalID(adjudication.Rationale) {
				return nil, fmt.Errorf("sample %q task %d requires independent adjudicator and rationale", sample.ID, taskIndex)
			}
			if err := validateAnnotation([]GoldLabel{adjudication.Label}, &Tasks{Items: []Task{sample.Tasks.Items[taskIndex]}}); err != nil {
				return nil, fmt.Errorf("sample %q task %d adjudication: %w", sample.ID, taskIndex, err)
			}
			consensus[taskIndex] = adjudication.Label
			adjudicated++
		}
		if nextAdjudication != len(sample.Adjudications) {
			return nil, fmt.Errorf("sample %q contains unnecessary or out-of-order adjudications", sample.ID)
		}
		entry := EvaluationSample{
			ID: sample.ID, Tasks: sample.Tasks, Gold: consensus, EvidenceRef: sample.Source.EvidenceRef,
			AnnotatorIDs: []string{first.AnnotatorID, second.AnnotatorID},
		}
		if sample.Split == SplitDevelopment {
			development.Samples = append(development.Samples, entry)
		} else {
			holdout.Samples = append(holdout.Samples, entry)
		}
	}
	if len(development.Samples) == 0 || len(holdout.Samples) == 0 {
		return nil, errors.New("curation requires nonempty development and holdout splits")
	}
	for _, dataset := range []*EvaluationDataset{development, holdout} {
		if err := validateEvaluationDataset(dataset); err != nil {
			return nil, fmt.Errorf("invalid curated evaluation dataset: %w", err)
		}
	}
	corpusJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal curated source: %w", err)
	}
	developmentJSON, err := json.Marshal(development)
	if err != nil {
		return nil, fmt.Errorf("marshal development dataset: %w", err)
	}
	holdoutJSON, err := json.Marshal(holdout)
	if err != nil {
		return nil, fmt.Errorf("marshal holdout dataset: %w", err)
	}
	return &FrozenCuration{
		Manifest: FreezeManifest{
			Version: input.Version, CorpusSHA256: digestJSON(corpusJSON),
			DevelopmentSHA256: digestJSON(developmentJSON), HoldoutSHA256: digestJSON(holdoutJSON),
			DevelopmentSamples: len(development.Samples), HoldoutSamples: len(holdout.Samples),
			AdjudicatedItems: adjudicated,
		},
		CorpusJSON: corpusJSON, DevelopmentJSON: developmentJSON, HoldoutJSON: holdoutJSON,
	}, nil
}

// VerifyFrozenCuration checks every recorded payload by re-deriving it from
// the frozen input and comparing byte-for-byte. The caller must pin the
// manifest digest separately to detect replacement of the entire bundle.
func VerifyFrozenCuration(frozen *FrozenCuration) error {
	if frozen == nil {
		return errors.New("frozen curation is nil")
	}
	var source CurationBatch
	if err := json.Unmarshal(frozen.CorpusJSON, &source); err != nil {
		return fmt.Errorf("decode source curation: %w", err)
	}
	derived, err := FreezeCuration(&source)
	if err != nil {
		return fmt.Errorf("invalid frozen source: %w", err)
	}
	if !reflect.DeepEqual(frozen.Manifest, derived.Manifest) ||
		!bytes.Equal(frozen.CorpusJSON, derived.CorpusJSON) ||
		!bytes.Equal(frozen.DevelopmentJSON, derived.DevelopmentJSON) ||
		!bytes.Equal(frozen.HoldoutJSON, derived.HoldoutJSON) {
		return errors.New("frozen curation checksum or derived dataset mismatch")
	}
	return nil
}

// ReadFrozenDataset returns a decoded copy. Holdout labels must be withheld
// from the model developer and any prompt-tuning process by external access
// controls, which this in-memory package cannot enforce.
func ReadFrozenDataset(frozen *FrozenCuration, split DatasetSplit) (*EvaluationDataset, error) {
	if err := VerifyFrozenCuration(frozen); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	switch split {
	case SplitDevelopment:
		raw = frozen.DevelopmentJSON
	case SplitHoldout:
		raw = frozen.HoldoutJSON
	default:
		return nil, fmt.Errorf("unsupported dataset split %q", split)
	}
	var output EvaluationDataset
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, err
	}
	return &output, nil
}
