package event

import (
	"testing"
	"time"
)

func TestTaskEvaluator(t *testing.T) {
	asOf := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)

	task := ResearchTask{
		ID:   "task-official",
		AsOf: asOf,
		Requirements: ResearchRequirements{
			PITSafe:               true,
			RequiredSourceTypes:   []SourceType{SourceOfficial},
			MinDocuments:          1,
			MinIndependentSources: 1,
			RequirePrimarySource:  true,
		},
	}

	doc := CanonicalDocument{
		ID: "doc-1",
		Source: DocumentSource{
			Publisher:  "上海证券交易所",
			URL:        "https://example.com/disclosure/1",
			SourceType: SourceOfficial,
			IsPrimary:  true,
		},
	}

	t.Run("satisfied even when another acquisition attempt failed", func(t *testing.T) {
		result := (TaskEvaluator{}).Evaluate(
			task,
			[]CanonicalDocument{doc},
			[]AcquisitionAttempt{
				{
					ID:     "attempt-failed",
					TaskID: task.ID,
					Method: AcquisitionWebSearch,
					Tool:   "web_search",
					Status: AcquisitionFailed,
				},
				{
					ID:     "attempt-api",
					TaskID: task.ID,
					Method: AcquisitionAPI,
					Tool:   "official_api",
					Status: AcquisitionSucceeded,
				},
			},
		)

		if result.Status != TaskSatisfied {
			t.Fatalf("status = %q, want %q; gaps=%v", result.Status, TaskSatisfied, result.Gaps)
		}
		if len(result.Attempts) != 2 {
			t.Fatalf("attempt refs = %v, want 2", result.Attempts)
		}
	})

	t.Run("partial when evidence exists but independent source requirement is not met", func(t *testing.T) {
		stronger := task
		stronger.Requirements.MinIndependentSources = 2

		result := (TaskEvaluator{}).Evaluate(
			stronger,
			[]CanonicalDocument{doc},
			nil,
		)

		if result.Status != TaskPartial {
			t.Fatalf("status = %q, want %q", result.Status, TaskPartial)
		}
		if !hasGap(result.Gaps, GapInsufficientIndependentSources) {
			t.Fatalf("gaps = %v, want independent-source gap", result.Gaps)
		}
	})

	t.Run("same publisher does not count as independent sources", func(t *testing.T) {
		stronger := task
		stronger.Requirements.MinDocuments = 2
		stronger.Requirements.MinIndependentSources = 2

		doc2 := doc
		doc2.ID = "doc-2"
		doc2.Source.URL = "https://example.com/disclosure/2"

		result := (TaskEvaluator{}).Evaluate(
			stronger,
			[]CanonicalDocument{doc, doc2},
			nil,
		)

		if result.Status != TaskPartial {
			t.Fatalf("status = %q, want %q", result.Status, TaskPartial)
		}
		if !hasGap(result.Gaps, GapInsufficientIndependentSources) {
			t.Fatalf("gaps = %v, want independent-source gap", result.Gaps)
		}
	})

	t.Run("syndicated identical content does not count as independent sources", func(t *testing.T) {
		stronger := task
		stronger.Requirements.MinDocuments = 2
		stronger.Requirements.MinIndependentSources = 2

		first := doc
		first.ContentHash = "same-content"

		second := doc
		second.ID = "doc-2"
		second.ContentHash = "same-content"
		second.Source.Publisher = "转载媒体"
		second.Source.URL = "https://mirror.example.com/article/1"
		second.Source.SourceType = SourceMainstreamMedia
		second.Source.IsPrimary = false

		result := (TaskEvaluator{}).Evaluate(
			stronger,
			[]CanonicalDocument{first, second},
			nil,
		)

		if result.Status != TaskPartial {
			t.Fatalf("status = %q, want %q", result.Status, TaskPartial)
		}
		if !hasGap(result.Gaps, GapInsufficientIndependentSources) {
			t.Fatalf("gaps = %v, want syndicated-content gap", result.Gaps)
		}
	})

	t.Run("unsatisfied when no usable document exists", func(t *testing.T) {
		result := (TaskEvaluator{}).Evaluate(task, nil, nil)

		if result.Status != TaskUnsatisfied {
			t.Fatalf("status = %q, want %q", result.Status, TaskUnsatisfied)
		}
		if !hasGap(result.Gaps, GapNoUsableDocuments) {
			t.Fatalf("gaps = %v, want no-usable-documents gap", result.Gaps)
		}
	})
}

func hasGap(gaps []ResearchGap, gapType ResearchGapType) bool {
	for _, gap := range gaps {
		if gap.Type == gapType {
			return true
		}
	}
	return false
}
