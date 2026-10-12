package technical

import (
 "context"
 "time"
)

// ToolObservation contains no request, response, credentials or provider error.
type ToolObservation struct {
 Status string
 At time.Time
 DurationMS int64
}

type toolObservationKey struct{}

// WithToolObserver is run-scoped and optional. Uninstrumented direct calls remain compatible.
func WithToolObserver(ctx context.Context, callback func(ToolObservation)) context.Context {
 if ctx == nil || callback == nil { return ctx }
 return context.WithValue(ctx, toolObservationKey{}, callback)
}

func reportTool(ctx context.Context, observation ToolObservation) {
 if ctx == nil { return }
 if cb, ok := ctx.Value(toolObservationKey{}).(func(ToolObservation)); ok && cb != nil {
  cb(observation)
 }
}
