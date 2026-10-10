package research

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

type progressKey struct{}
type progressWriter struct {
	mu  sync.Mutex
	out io.Writer
}
type progressCallbackKey struct{}

// ProgressEvent is safe stage metadata. It deliberately omits prompts,
// model responses, portfolio values and provider credentials.
type ProgressEvent struct {
	Stage      string    `json:"stage"`
	Status     string    `json:"status"`
	At         time.Time `json:"at"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

// WithProgressEvents registers an optional callback for typed stage metadata.
// Callbacks must be thread-safe, bounded and must not expose sensitive data.
func WithProgressEvents(ctx context.Context, callback func(ProgressEvent)) context.Context {
	if ctx == nil || callback == nil {
		return ctx
	}
	return context.WithValue(ctx, progressCallbackKey{}, callback)
}

// WithProgress keeps the original opt-in human-readable stderr progress path.
func WithProgress(ctx context.Context, out io.Writer) context.Context {
	if out == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey{}, &progressWriter{out: out})
}

func publishProgress(ctx context.Context, ev ProgressEvent) {
	if cb, ok := ctx.Value(progressCallbackKey{}).(func(ProgressEvent)); ok && cb != nil {
		cb(ev)
	}
	if w, ok := ctx.Value(progressKey{}).(*progressWriter); ok && w != nil {
		w.mu.Lock()
		if ev.Status == "START" {
			fmt.Fprintf(w.out, "[Aegis] stage=%s status=START\n", ev.Stage)
		} else {
			fmt.Fprintf(w.out, "[Aegis] stage=%s status=%s duration=%s\n", ev.Stage, ev.Status, time.Duration(ev.DurationMS)*time.Millisecond)
		}
		w.mu.Unlock()
	}
}

// ReportProgress is intended for trusted workflow adapters that need to emit
// a stage event outside startProgress (e.g., lifecycle/runner integration).
func ReportProgress(ctx context.Context, event ProgressEvent) {
	if ctx != nil && event.Stage != "" && (event.Status == "START" || event.Status == "DONE" || event.Status == "FAIL") {
		if event.At.IsZero() {
			event.At = time.Now().UTC()
		}
		publishProgress(ctx, event)
	}
}

func startProgress(ctx context.Context, stage string) func(*error) {
	if ctx == nil {
		return func(*error) {}
	}
	_, hasSink := ctx.Value(progressCallbackKey{}).(func(ProgressEvent))
	_, hasWriter := ctx.Value(progressKey{}).(*progressWriter)
	if !hasSink && !hasWriter {
		return func(*error) {}
	}
	start := time.Now()
	publishProgress(ctx, ProgressEvent{Stage: stage, Status: "START", At: start.UTC()})
	return func(err *error) {
		status := "DONE"
		if err != nil && *err != nil {
			status = "FAIL"
		}
		publishProgress(ctx, ProgressEvent{Stage: stage, Status: status, At: time.Now().UTC(), DurationMS: time.Since(start).Milliseconds()})
	}
}
