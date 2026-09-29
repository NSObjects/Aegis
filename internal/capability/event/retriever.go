package event

import "context"

// Retriever is the smallest capability contract for one concrete retrieval
// implementation. Routing across multiple retrievers is intentionally deferred
// until a second real implementation exists.
type Retriever interface {
	ID() string
	Method() AcquisitionMethod
	Retrieve(
		ctx context.Context,
		task ResearchTask,
	) ([]RawDocument, error)
}
