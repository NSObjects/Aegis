package event

import (
	"context"
	"errors"
)

type SnapshotRetriever struct {
	Store EvidenceSnapshotStore
}

var _ Retriever = (*SnapshotRetriever)(nil)

func (*SnapshotRetriever) ID() string {
	return "evidence_snapshot_store"
}

func (*SnapshotRetriever) Method() AcquisitionMethod {
	return AcquisitionSnapshotStore
}

func (r *SnapshotRetriever) Retrieve(
	ctx context.Context,
	task ResearchTask,
) ([]RawDocument, error) {
	if r == nil || r.Store == nil {
		return nil, errors.New(
			"evidence snapshot retriever store is required",
		)
	}
	if task.AsOf.IsZero() {
		return nil, errors.New(
			"evidence snapshot retriever requires task as_of",
		)
	}

	return r.Store.Query(
		ctx,
		EvidenceSnapshotQuery{
			AsOf:          task.AsOf,
			TimeRange:     task.TimeRange,
			Symbol:        task.Symbol,
			SourceTypes:   task.Requirements.RequiredSourceTypes,
			Authorities:   task.Requirements.RequiredAuthorities,
			EvidenceKinds: task.Requirements.RequiredEvidenceKinds,
		},
	)
}
