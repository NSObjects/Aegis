package event

import "context"

// Retriever is the smallest capability contract for one concrete retrieval
// implementation. Multi-source orchestration belongs to AcquisitionPlanRunner;
// TaskRunner and Retriever remain leaf-level contracts.
type Retriever interface {
	ID() string
	Method() AcquisitionMethod
	Retrieve(
		ctx context.Context,
		task ResearchTask,
	) ([]RawDocument, error)
}
