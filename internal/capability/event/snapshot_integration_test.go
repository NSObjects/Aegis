package event

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type snapshotStoreSpy struct {
	saved     []RawDocument
	saveError error
	queryDocs []RawDocument
	query     EvidenceSnapshotQuery
}

func (s *snapshotStoreSpy) Save(
	_ context.Context,
	documents []RawDocument,
) error {
	s.saved = append(
		s.saved,
		documents...,
	)
	return s.saveError
}

func (s *snapshotStoreSpy) Query(
	_ context.Context,
	query EvidenceSnapshotQuery,
) ([]RawDocument, error) {
	s.query = query
	return append(
		[]RawDocument(nil),
		s.queryDocs...,
	), nil
}

func TestTaskRunnerPersistsBeforePITFiltering(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	published := asOf.Add(-time.Hour)
	observed := asOf.Add(time.Hour)

	store := &snapshotStoreSpy{}
	output, err := (TaskRunner{
		Now: func() time.Time {
			return observed
		},
		SnapshotStore: store,
	}).Run(
		context.Background(),
		ResearchTask{
			ID:   "task-persist-before-pit",
			AsOf: asOf,
			Requirements: ResearchRequirements{
				PITSafe: true,
			},
		},
		fakeRetriever{
			id:     "mutable-news",
			method: AcquisitionAPI,
			docs: []RawDocument{
				{
					ID:          "news-later",
					Title:       "old article current body",
					Content:     "observed after historical as_of",
					PublishedAt: &published,
					AvailableAt: &observed,
					RetrievedAt: observed,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(store.saved) != 1 {
		t.Fatalf(
			"saved snapshots = %d, want 1",
			len(store.saved),
		)
	}
	if store.saved[0].TaskID != "task-persist-before-pit" ||
		store.saved[0].AttemptID == "" {
		t.Fatalf(
			"saved document was not normalized first: %+v",
			store.saved[0],
		)
	}
	if len(output.Canonicalization.Documents) != 0 ||
		len(output.Canonicalization.Rejected) != 1 {
		t.Fatalf(
			"canonicalization = %+v, want PIT rejection after persistence",
			output.Canonicalization,
		)
	}
	if output.Canonicalization.Rejected[0].Reason !=
		RejectionFutureAvailableAt {
		t.Fatalf(
			"rejection = %q, want %q",
			output.Canonicalization.Rejected[0].Reason,
			RejectionFutureAvailableAt,
		)
	}
}

func TestTaskRunnerSnapshotFailureIsStructural(
	t *testing.T,
) {
	store := &snapshotStoreSpy{
		saveError: errors.New(
			"disk unavailable",
		),
	}

	output, err := (TaskRunner{
		SnapshotStore: store,
	}).Run(
		context.Background(),
		ResearchTask{
			ID: "task-store-failure",
		},
		fakeRetriever{
			id:     "source",
			method: AcquisitionAPI,
			docs: []RawDocument{
				{
					ID:          "document",
					Title:       "document",
					RetrievedAt: time.Now().UTC(),
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"persist evidence snapshots",
		) {
		t.Fatalf(
			"Run() error = %v, want persistence structural error",
			err,
		)
	}
	if len(output.RawDocuments) != 1 {
		t.Fatalf(
			"raw documents = %d, want retrieved document preserved",
			len(output.RawDocuments),
		)
	}
}

func TestAcquisitionPlanRunnerPropagatesSnapshotStore(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	published := asOf.Add(-time.Hour)
	store := &snapshotStoreSpy{}

	_, err := (AcquisitionPlanRunner{
		Now: func() time.Time {
			return asOf
		},
		SnapshotStore: store,
	}).Run(
		context.Background(),
		ResearchTask{
			ID:   "root-snapshot-propagation",
			AsOf: asOf,
			Requirements: ResearchRequirements{
				PITSafe: true,
			},
		},
		AcquisitionPlan{
			Steps: []AcquisitionStep{
				{
					ID:    "direct",
					Scope: AcquisitionScopeDirect,
					Task: ResearchTask{},
					Retriever: fakeRetriever{
						id:     "direct-source",
						method: AcquisitionAPI,
						docs: []RawDocument{
							{
								ID:          "direct-document",
								Title:       "direct",
								PublishedAt: &published,
								AvailableAt: &published,
							},
						},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"AcquisitionPlanRunner.Run() error = %v",
			err,
		)
	}
	if len(store.saved) != 1 {
		t.Fatalf(
			"saved snapshots = %d, want 1",
			len(store.saved),
		)
	}
}

func TestSnapshotRetrieverQueriesStoreUsingTaskContract(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	store := &snapshotStoreSpy{
		queryDocs: []RawDocument{
			{
				ID: "stored-document",
			},
		},
	}
	retriever := &SnapshotRetriever{
		Store: store,
	}

	var _ Retriever = retriever

	documents, err := retriever.Retrieve(
		context.Background(),
		ResearchTask{
			ID:     "snapshot-task",
			Symbol: "600519.SH",
			AsOf:   asOf,
			TimeRange: TimeRange{
				Start: asOf.Add(-24 * time.Hour),
				End:   asOf,
			},
			Requirements: ResearchRequirements{
				RequiredSourceTypes: []SourceType{
					SourceOfficial,
				},
				RequiredAuthorities: []EvidenceAuthority{
					AuthorityIssuer,
				},
				RequiredEvidenceKinds: []EvidenceKind{
					EvidenceDisclosure,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"Retrieve() error = %v",
			err,
		)
	}
	if retriever.Method() !=
		AcquisitionSnapshotStore {
		t.Fatalf(
			"method = %q, want %q",
			retriever.Method(),
			AcquisitionSnapshotStore,
		)
	}
	if len(documents) != 1 {
		t.Fatalf(
			"documents = %d, want 1",
			len(documents),
		)
	}
	if store.query.Symbol != "600519.SH" ||
		!store.query.AsOf.Equal(asOf) {
		t.Fatalf(
			"query = %+v",
			store.query,
		)
	}
	if len(store.query.SourceTypes) != 1 ||
		store.query.SourceTypes[0] !=
			SourceOfficial {
		t.Fatalf(
			"source filters = %v",
			store.query.SourceTypes,
		)
	}
}
