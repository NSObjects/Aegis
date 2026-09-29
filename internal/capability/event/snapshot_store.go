package event

import (
	"context"
	"time"
)

// EvidenceSnapshot is one immutable observation of one logical-document
// version. Multiple observations may point at the same VersionID.
type EvidenceSnapshot struct {
	ID                string      `json:"snapshot_id"`
	LogicalDocumentID string      `json:"logical_document_id"`
	VersionID         string      `json:"version_id"`
	Document          RawDocument `json:"document"`
}

// EvidenceSnapshotQuery describes a PIT-safe lookup over persisted evidence.
// Filters within one dimension are OR'ed; dimensions are combined with AND.
type EvidenceSnapshotQuery struct {
	AsOf time.Time

	TimeRange TimeRange
	Symbol    string

	SourceTypes   []SourceType
	Authorities   []EvidenceAuthority
	EvidenceKinds []EvidenceKind
}

type EvidenceSnapshotStore interface {
	Save(
		ctx context.Context,
		documents []RawDocument,
	) error

	Query(
		ctx context.Context,
		query EvidenceSnapshotQuery,
	) ([]RawDocument, error)
}
