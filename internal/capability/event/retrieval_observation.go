package event

import (
 "context"
 "time"
)

// RetrievalObservation reports only actual retriever execution metadata.
// No source document, query, account, URL, provider error or API key is exposed.
type RetrievalObservation struct {
 RetrieverID string
 Status string
 At time.Time
 DurationMS int64
}

type retrievalObserverKey struct{}

// WithRetrievalObserver attaches a run-scoped observer without changing the
// Retriever interface or the evidence acquisition output contract.
func WithRetrievalObserver(ctx context.Context, callback func(RetrievalObservation)) context.Context {
 if ctx == nil || callback == nil { return ctx }
 return context.WithValue(ctx, retrievalObserverKey{}, callback)
}

func reportRetrieval(ctx context.Context, event RetrievalObservation) {
 if ctx == nil { return }
 if cb, ok := ctx.Value(retrievalObserverKey{}).(func(RetrievalObservation)); ok && cb != nil {
  cb(event)
 }
}
