package event

import (
	"context"
	"errors"
	"strings"
)

type SnapshotRetriever struct {
	Store EvidenceSnapshotStore

	Name          string
	SourceTypes   []SourceType
	Authorities   []EvidenceAuthority
	EvidenceKinds []EvidenceKind
}

var _ Retriever = (*SnapshotRetriever)(nil)

func (r *SnapshotRetriever) ID() string {
	if r != nil {
		if name := strings.TrimSpace(
			r.Name,
		); name != "" {
			return name
		}
	}
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

	sourceTypes := r.SourceTypes
	if len(sourceTypes) == 0 {
		sourceTypes =
			task.Requirements.RequiredSourceTypes
	}
	authorities := r.Authorities
	if len(authorities) == 0 {
		authorities =
			task.Requirements.RequiredAuthorities
	}
	evidenceKinds := r.EvidenceKinds
	if len(evidenceKinds) == 0 {
		evidenceKinds =
			task.Requirements.RequiredEvidenceKinds
	}

	return r.Store.Query(
		ctx,
		EvidenceSnapshotQuery{
			AsOf:          task.AsOf,
			TimeRange:     task.TimeRange,
			Symbol:        task.Symbol,
			SourceTypes:   sourceTypes,
			Authorities:   authorities,
			EvidenceKinds: evidenceKinds,
		},
	)
}
