package semanticreview

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// DatasetProvenance is a caller-asserted provenance class. HUMAN_REVIEWED is
// not independently authenticated by this offline library.
type DatasetProvenance string

const (
	ProvenanceSynthetic     DatasetProvenance = "SYNTHETIC"
	ProvenanceHumanReviewed DatasetProvenance = "HUMAN_REVIEWED"
)

// EvaluationSample contains independent reference labels for one complete
// review task collection. Gold labels must not come from model predictions.
type EvaluationSample struct {
	ID           string      `json:"id"`
	Tasks        *Tasks      `json:"tasks"`
	Gold         []GoldLabel `json:"gold"`
	EvidenceRef  string      `json:"evidence_ref,omitempty"`
	AnnotatorIDs []string    `json:"annotator_ids,omitempty"`
}

type EvaluationDataset struct {
	Version    string            `json:"version"`
	Provenance DatasetProvenance `json:"provenance"`
	Samples    []EvaluationSample `json:"samples"`
}

// FailureCode explicitly distinguishes an unavailable review from an
// actual reviewer verdict of UNCERTAIN.
type FailureCode string

const (
	FailureTimeout       FailureCode = "TIMEOUT"
	FailureModelError    FailureCode = "MODEL_ERROR"
	FailureInvalidReport FailureCode = "INVALID_REPORT"
)

type EvaluationPrediction struct {
	SampleID     string   `json:"sample_id"`
	Report       *Report  `json:"report,omitempty"`
	FailureCode  FailureCode `json:"failure_code,omitempty"`
	LatencyMs    *int64   `json:"latency_ms,omitempty"`
	InputTokens  *int64   `json:"input_tokens,omitempty"`
	OutputTokens *int64   `json:"output_tokens,omitempty"`
}

type EvaluationRun struct {
	DatasetVersion string                 `json:"dataset_version"`
	RunID          string                 `json:"run_id"`
	ModelID        string                 `json:"model_id"`
	Samples        []EvaluationPrediction `json:"samples"`
}

// EvaluationSummary is descriptive, not itself trusted as evidence for a
// policy pass: CheckQuality always recomputes directly from the input run.
type EvaluationSummary struct {
	DatasetVersion string            `json:"dataset_version"`
	RunID          string            `json:"run_id"`
	ModelID        string            `json:"model_id"`
	Provenance     DatasetProvenance `json:"provenance"`
	TotalSamples       int           `json:"total_samples"`
	CompletedSamples   int           `json:"completed_samples"`
	UnavailableSamples int           `json:"unavailable_samples"`
	LabeledItems       int           `json:"labeled_items"`
	EvaluatedItems     int           `json:"evaluated_items"`
	ExactMatches       int           `json:"exact_matches"`
	PredictedUncertain int           `json:"predicted_uncertain"`
	FalseConsistentOnContradictory int `json:"false_consistent_on_contradictory"`
	FalseContradictoryOnConsistent int `json:"false_contradictory_on_consistent"`
	ObservedLatencyCount int         `json:"observed_latency_count"`
	P95LatencyMs *int64             `json:"p95_latency_ms,omitempty"`
	ObservedTokenCount int           `json:"observed_token_count"`
	ObservedInputTokens int64        `json:"observed_input_tokens"`
	ObservedOutputTokens int64       `json:"observed_output_tokens"`
}

// EvaluateRun never calls a model or contacts a provider. All input predictions
// and measurements are captured offline by a separate caller.
func EvaluateRun(dataset *EvaluationDataset, run *EvaluationRun) (*EvaluationSummary, error) {
	if err := validateEvaluationDataset(dataset); err != nil {
		return nil, err
	}
	if run == nil {
		return nil, errors.New("evaluation run is nil")
	}
	if strings.TrimSpace(run.DatasetVersion) != dataset.Version {
		return nil, fmt.Errorf("evaluation run dataset version mismatch")
	}
	if strings.TrimSpace(run.RunID) == "" || strings.TrimSpace(run.ModelID) == "" {
		return nil, errors.New("evaluation run_id and model_id are required")
	}
	if len(run.Samples) != len(dataset.Samples) {
		return nil, fmt.Errorf("evaluation prediction count %d differs from dataset sample count %d", len(run.Samples), len(dataset.Samples))
	}
	result := &EvaluationSummary{
		DatasetVersion: dataset.Version, RunID: run.RunID,
		ModelID: run.ModelID, Provenance: dataset.Provenance,
		TotalSamples: len(dataset.Samples),
	}
	var measuredLatencies []int64
	for i, sample := range dataset.Samples {
		prediction := run.Samples[i]
		if prediction.SampleID != sample.ID {
			return nil, fmt.Errorf("evaluation sample_id at index %d is %q, expected %q", i, prediction.SampleID, sample.ID)
		}
		result.LabeledItems += len(sample.Gold)
		if prediction.LatencyMs != nil {
			if *prediction.LatencyMs < 0 {
				return nil, fmt.Errorf("sample %q latency must be nonnegative", sample.ID)
			}
			measuredLatencies = append(measuredLatencies, *prediction.LatencyMs)
			result.ObservedLatencyCount++
		}
		if (prediction.InputTokens == nil) != (prediction.OutputTokens == nil) {
			return nil, fmt.Errorf("sample %q token metrics must be supplied together", sample.ID)
		}
		if prediction.InputTokens != nil {
			if *prediction.InputTokens < 0 || *prediction.OutputTokens < 0 {
				return nil, fmt.Errorf("sample %q token counts must be nonnegative", sample.ID)
			}
			result.ObservedTokenCount++
			result.ObservedInputTokens += *prediction.InputTokens
			result.ObservedOutputTokens += *prediction.OutputTokens
			if result.ObservedInputTokens < 0 || result.ObservedOutputTokens < 0 {
				return nil, fmt.Errorf("sample %q token aggregation overflow", sample.ID)
			}
		}
		if prediction.Report != nil && prediction.FailureCode != "" {
			return nil, fmt.Errorf("sample %q has both report and failure_code", sample.ID)
		}
		if prediction.Report == nil {
			switch prediction.FailureCode {
			case FailureTimeout, FailureModelError, FailureInvalidReport:
				result.UnavailableSamples++
			case "":
				return nil, fmt.Errorf("sample %q missing report or failure code", sample.ID)
			default:
				return nil, fmt.Errorf("sample %q has unsupported failure code %q", sample.ID, prediction.FailureCode)
			}
			continue
		}
		score, err := ScoreLabeledReport(sample.Tasks, prediction.Report, sample.Gold)
		if err != nil {
			return nil, fmt.Errorf("sample %q: %w", sample.ID, err)
		}
		result.CompletedSamples++
		result.EvaluatedItems += score.Total
		result.ExactMatches += score.ExactMatches
		result.PredictedUncertain += score.PredictedUncertain
		result.FalseConsistentOnContradictory += score.FalseConsistentOnContradictory
		result.FalseContradictoryOnConsistent += score.FalseContradictoryOnConsistent
	}
	if len(measuredLatencies) > 0 {
		sort.Slice(measuredLatencies, func(i, j int) bool { return measuredLatencies[i] < measuredLatencies[j] })
		// Nearest-rank p95: ceil(0.95 * n), using integer arithmetic.
		index := (95*len(measuredLatencies) + 99)/100 - 1
		value := measuredLatencies[index]
		result.P95LatencyMs = &value
	}
	return result, nil
}

func validateEvaluationDataset(dataset *EvaluationDataset) error {
	if dataset == nil {
		return errors.New("evaluation dataset is nil")
	}
	if strings.TrimSpace(dataset.Version) == "" {
		return errors.New("evaluation dataset version is required")
	}
	switch dataset.Provenance {
	case ProvenanceSynthetic, ProvenanceHumanReviewed:
	default:
		return fmt.Errorf("unsupported dataset provenance %q", dataset.Provenance)
	}
	if len(dataset.Samples) == 0 {
		return errors.New("evaluation dataset requires samples")
	}
	seen := make(map[string]struct{}, len(dataset.Samples))
	for i, sample := range dataset.Samples {
		id := strings.TrimSpace(sample.ID)
		if id == "" || id != sample.ID {
			return fmt.Errorf("sample %d has invalid sample_id", i)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("sample %d has duplicate sample_id %q", i, id)
		}
		seen[id] = struct{}{}
		if sample.Tasks == nil || len(sample.Tasks.Items) == 0 || strings.TrimSpace(sample.Tasks.Symbol) == "" {
			return fmt.Errorf("sample %q must contain nonempty Tasks and symbol", id)
		}
		if len(sample.Gold) != len(sample.Tasks.Items) {
			return fmt.Errorf("sample %q gold label count mismatch", id)
		}
		seenTaskIDs := make(map[string]struct{}, len(sample.Tasks.Items))
		for j, gold := range sample.Gold {
			task := sample.Tasks.Items[j]
			key := fmt.Sprintf("%d/%s", task.InsightIndex, task.ArgumentID)
			if _, duplicate := seenTaskIDs[key]; duplicate {
				return fmt.Errorf("sample %q has duplicate task identity at index %d", id, j)
			}
			seenTaskIDs[key] = struct{}{}
			if gold.InsightIndex != task.InsightIndex || gold.ArgumentID != task.ArgumentID || strings.TrimSpace(gold.ArgumentID) == "" {
				return fmt.Errorf("sample %q gold label %d argument_id or insight_index mismatch", id, j)
			}
			switch gold.ExpectedVerdict {
			case VerdictConsistent, VerdictContradictory, VerdictUncertain:
			default:
				return fmt.Errorf("sample %q gold label %d has invalid verdict", id, j)
			}
		}
		if dataset.Provenance == ProvenanceHumanReviewed {
			if strings.TrimSpace(sample.EvidenceRef) == "" {
				return fmt.Errorf("human-reviewed sample %q missing evidence reference", id)
			}
			reviewers := make(map[string]struct{}, len(sample.AnnotatorIDs))
			for _, reviewer := range sample.AnnotatorIDs {
				name := strings.TrimSpace(reviewer)
				if name == "" || name != reviewer {
					return fmt.Errorf("human-reviewed sample %q invalid annotator id", id)
				}
				reviewers[name] = struct{}{}
			}
			if len(reviewers) < 2 {
				return fmt.Errorf("human-reviewed sample %q needs two distinct annotators", id)
			}
		}
	}
	return nil
}

// QualityPolicy is for OFFLINE benchmark acceptance, never a research or
// trading authorization. All rates use integer basis points (0..10000).
type QualityPolicy struct {
	MinLabeledItems int `json:"min_labeled_items"`
	MinAvailabilityBps int `json:"min_availability_bps"`
	MinExactMatchBps int `json:"min_exact_match_bps"`
	MaxFalseConsistentOnContradictory int `json:"max_false_consistent_on_contradictory"`
}

type QualityStatus string

const (
	QualityPass QualityStatus = "PASS"
	QualityFail QualityStatus = "FAIL"
	QualityNotEligible QualityStatus = "NOT_ELIGIBLE"
)

type QualityDecision struct {
	Status QualityStatus `json:"status"`
	Reasons []string `json:"reasons,omitempty"`
	Metrics *EvaluationSummary `json:"metrics"`
}

// CheckQuality independently recomputes metrics to prevent callers from
// handing in a forged/edited EvaluationSummary. Dataset provenance is still
// an unauthenticated claim: PASS means thresholds met on caller-supplied
// reviewed data, not that independent human review was verified.
func CheckQuality(dataset *EvaluationDataset, run *EvaluationRun, policy QualityPolicy) (*QualityDecision, error) {
	if policy.MinLabeledItems < 1 || policy.MaxFalseConsistentOnContradictory < 0 ||
		policy.MinAvailabilityBps < 0 || policy.MinAvailabilityBps > 10000 ||
		policy.MinExactMatchBps < 0 || policy.MinExactMatchBps > 10000 {
		return nil, errors.New("invalid offline quality policy")
	}
	metrics, err := EvaluateRun(dataset, run)
	if err != nil { return nil, err }
	decision := &QualityDecision{Status: QualityNotEligible, Metrics: metrics}
	if dataset.Provenance != ProvenanceHumanReviewed {
		decision.Reasons = []string{"SYNTHETIC_DATASET"}
		return decision, nil
	}
	if metrics.LabeledItems < policy.MinLabeledItems || metrics.EvaluatedItems == 0 {
		decision.Reasons = []string{"INSUFFICIENT_SAMPLE_SIZE"}
		return decision, nil
	}
	decision.Status = QualityPass
	// Multiplication avoids rounding a 2/3 sample coverage (6666.67 bps)
	// into the wrong integer boundary.
	if int64(metrics.CompletedSamples)*10000 < int64(policy.MinAvailabilityBps)*int64(metrics.TotalSamples) {
		decision.Reasons = append(decision.Reasons, "LOW_AVAILABILITY")
	}
	if int64(metrics.ExactMatches)*10000 < int64(policy.MinExactMatchBps)*int64(metrics.EvaluatedItems) {
		decision.Reasons = append(decision.Reasons, "LOW_AGREEMENT")
	}
	if metrics.FalseConsistentOnContradictory > policy.MaxFalseConsistentOnContradictory {
		decision.Reasons = append(decision.Reasons, "FALSE_CONSISTENT_LIMIT")
	}
	if len(decision.Reasons) != 0 {
		decision.Status = QualityFail
	}
	return decision, nil
}
