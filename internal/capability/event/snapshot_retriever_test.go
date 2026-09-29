package event

import (
	"context"
	"testing"
	"time"
)

type recordingSnapshotStore struct {
	query EvidenceSnapshotQuery
	docs  []RawDocument
}

func (s *recordingSnapshotStore) Save(
	context.Context,
	[]RawDocument,
) error {
	return nil
}

func (s *recordingSnapshotStore) Query(
	_ context.Context,
	query EvidenceSnapshotQuery,
) ([]RawDocument, error) {
	s.query = query
	return append(
		[]RawDocument(nil),
		s.docs...,
	), nil
}

func TestSnapshotRetrieverUsesFixedChannelFilters(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 12, 0, 0, 0,
		time.UTC,
	)
	store := &recordingSnapshotStore{}

	retriever := &SnapshotRetriever{
		Store: store,
		Name:  "snapshot_news",
		SourceTypes: []SourceType{
			SourceIndustryMedia,
			SourceMainstreamMedia,
		},
		Authorities: []EvidenceAuthority{
			AuthorityMedia,
		},
		EvidenceKinds: []EvidenceKind{
			EvidenceNewsReport,
		},
	}

	if got := retriever.ID();
		got != "snapshot_news" {
		t.Fatalf(
			"ID() = %q, want snapshot_news",
			got,
		)
	}

	_, err := retriever.Retrieve(
		context.Background(),
		ResearchTask{
			ID:     "task",
			Symbol: "",
			AsOf:   asOf,
			Requirements: ResearchRequirements{
				// These aggregate requirements must not replace a
				// channel's fixed snapshot filters.
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

	if !store.query.AsOf.Equal(asOf) {
		t.Fatalf(
			"query as_of = %s, want %s",
			store.query.AsOf,
			asOf,
		)
	}
	if len(store.query.SourceTypes) != 2 ||
		store.query.SourceTypes[0] !=
			SourceIndustryMedia ||
		store.query.SourceTypes[1] !=
			SourceMainstreamMedia {
		t.Fatalf(
			"source types = %+v",
			store.query.SourceTypes,
		)
	}
	if len(store.query.Authorities) != 1 ||
		store.query.Authorities[0] !=
			AuthorityMedia {
		t.Fatalf(
			"authorities = %+v",
			store.query.Authorities,
		)
	}
	if len(store.query.EvidenceKinds) != 1 ||
		store.query.EvidenceKinds[0] !=
			EvidenceNewsReport {
		t.Fatalf(
			"evidence kinds = %+v",
			store.query.EvidenceKinds,
		)
	}
}

func TestSnapshotRetrieverFallsBackToTaskFilters(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 12, 0, 0, 0,
		time.UTC,
	)
	store := &recordingSnapshotStore{}

	retriever := &SnapshotRetriever{
		Store: store,
	}
	_, err := retriever.Retrieve(
		context.Background(),
		ResearchTask{
			ID:     "task",
			Symbol: "600519.SH",
			AsOf:   asOf,
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

	if len(store.query.SourceTypes) != 1 ||
		store.query.SourceTypes[0] !=
			SourceOfficial {
		t.Fatalf(
			"source types = %+v",
			store.query.SourceTypes,
		)
	}
	if store.query.Symbol != "600519.SH" {
		t.Fatalf(
			"symbol = %q",
			store.query.Symbol,
		)
	}
}
