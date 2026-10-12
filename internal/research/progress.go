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
type toolCallbackKey struct{}

// ToolObservation contains only explicitly observed, non-sensitive tool metadata.
// Call sites must not attach arguments, results, URLs, credentials or prompts.
type ToolObservation struct {
  Stage string
  Tool string
  Status string
  Outcome string // optional fixed execution outcome; no provider error details
  At time.Time
  DurationMS int64
}

// WithToolObservations installs a run-scoped optional sink; no tool calls are inferred.
func WithToolObservations(ctx context.Context, callback func(ToolObservation)) context.Context {
  if ctx == nil || callback == nil { return ctx }
  return context.WithValue(ctx, toolCallbackKey{}, callback)
}

// ReportToolObservation accepts only fixed known stages and tightly restricted identifiers.
// Unknown values are dropped rather than persisted as attacker-controlled strings.
func ReportToolObservation(ctx context.Context, ev ToolObservation) {
  if ctx == nil || (ev.Status != "START" && ev.Status != "DONE" && ev.Status != "FAIL") || ev.DurationMS < 0 { return }
  switch ev.Stage {
  case "Market", "Technical", "Event", "Fundamental", "Synthesis", "Debate", "ResearchManager", "Trading", "Risk", "Portfolio":
  default: return
  }
  if len(ev.Tool) == 0 || len(ev.Tool) > 64 { return }
  for _, ch := range ev.Tool {
    if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') { return }
  }
  if ev.Outcome != "" && ev.Outcome != "SUCCEEDED" && ev.Outcome != "PARTIAL" && ev.Outcome != "TIMEOUT" && ev.Outcome != "FAILED" { return }
  if ev.At.IsZero() { ev.At = time.Now().UTC() }
  if cb, ok := ctx.Value(toolCallbackKey{}).(func(ToolObservation)); ok && cb != nil { cb(ev) }
}


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
