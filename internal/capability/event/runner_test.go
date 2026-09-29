package event

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRetriever struct {
	id     string
	method AcquisitionMethod
	docs   []RawDocument
	err    error
}

func (r fakeRetriever) ID() string {
	return r.id
}

func (r fakeRetriever) Method() AcquisitionMethod {
	return r.method
}

func (r fakeRetriever) Retrieve(
	_ context.Context,
	_ ResearchTask,
) ([]RawDocument, error) {
	return r.docs, r.err
}

func TestTaskRunner(t *testing.T) {
	asOf := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	publishedAt := asOf.Add(-time.Hour)
	retrievedAt := asOf.Add(24 * time.Hour)

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

	t.Run("runs one retriever through canonicalization and evaluation", func(t *testing.T) {
		runner := TaskRunner{
			Now: func() time.Time {
				return retrievedAt
			},
		}

		output, err := runner.Run(
			context.Background(),
			task,
			fakeRetriever{
				id:     "official_api",
				method: AcquisitionAPI,
				docs: []RawDocument{
					{
						Title:       "贵州茅台公告",
						Content:     "official disclosure",
						URL:         "https://example.com/disclosure/1",
						Publisher:   "上海证券交易所",
						SourceType:  SourceOfficial,
						IsPrimary:   true,
						PublishedAt: &publishedAt,
					},
				},
			},
		)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if output.Result.Status != TaskSatisfied {
			t.Fatalf("status = %q, want %q", output.Result.Status, TaskSatisfied)
		}
		if output.Attempt.Status != AcquisitionSucceeded {
			t.Fatalf("attempt status = %q, want %q", output.Attempt.Status, AcquisitionSucceeded)
		}
		if len(output.RawDocuments) != 1 {
			t.Fatalf("raw documents = %d, want 1", len(output.RawDocuments))
		}
		if output.RawDocuments[0].ID == "" {
			t.Fatal("runner should assign raw document id")
		}
		if output.RawDocuments[0].TaskID != task.ID {
			t.Fatalf("raw task id = %q, want %q", output.RawDocuments[0].TaskID, task.ID)
		}
		if output.RawDocuments[0].AttemptID != output.Attempt.ID {
			t.Fatalf(
				"raw attempt id = %q, want %q",
				output.RawDocuments[0].AttemptID,
				output.Attempt.ID,
			)
		}
		if len(output.Canonicalization.Documents) != 1 {
			t.Fatalf(
				"canonical documents = %d, want 1",
				len(output.Canonicalization.Documents),
			)
		}
		if len(output.Attempt.DocumentRefs) != 1 {
			t.Fatalf("attempt document refs = %v, want 1", output.Attempt.DocumentRefs)
		}
	})

	t.Run("retriever failure becomes task result instead of runner failure", func(t *testing.T) {
		runner := TaskRunner{
			Now: func() time.Time {
				return retrievedAt
			},
		}

		output, err := runner.Run(
			context.Background(),
			task,
			fakeRetriever{
				id:     "official_api",
				method: AcquisitionAPI,
				err:    errors.New("provider unavailable"),
			},
		)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if output.Attempt.Status != AcquisitionFailed {
			t.Fatalf("attempt status = %q, want %q", output.Attempt.Status, AcquisitionFailed)
		}
		if output.Attempt.ErrorMessage == "" {
			t.Fatal("attempt should record retriever error")
		}
		if output.Result.Status != TaskUnsatisfied {
			t.Fatalf("status = %q, want %q", output.Result.Status, TaskUnsatisfied)
		}
	})

	t.Run("usable documents survive a partial retriever failure", func(t *testing.T) {
		runner := TaskRunner{
			Now: func() time.Time {
				return retrievedAt
			},
		}

		output, err := runner.Run(
			context.Background(),
			task,
			fakeRetriever{
				id:     "official_api",
				method: AcquisitionAPI,
				docs: []RawDocument{
					{
						Title:       "贵州茅台公告",
						Content:     "official disclosure",
						URL:         "https://example.com/disclosure/1",
						Publisher:   "上海证券交易所",
						SourceType:  SourceOfficial,
						IsPrimary:   true,
						PublishedAt: &publishedAt,
					},
				},
				err: errors.New("second page failed"),
			},
		)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if output.Attempt.Status != AcquisitionPartial {
			t.Fatalf("attempt status = %q, want %q", output.Attempt.Status, AcquisitionPartial)
		}
		if output.Result.Status != TaskSatisfied {
			t.Fatalf("status = %q, want %q", output.Result.Status, TaskSatisfied)
		}
	})
}
