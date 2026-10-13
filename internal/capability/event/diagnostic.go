package event

import (
	"context"
	"errors"
	"strings"
)

// DiagnosticObservation is safe execution metadata for diagnosing event
// acquisition. It deliberately excludes request/response bodies, URLs,
// credentials and provider error text.
type DiagnosticObservation struct {
	Component    string
	Operation    string
	Tool         string
	Status       string
	Code         string
	Attempt      int
	Batch        int
	InputCount   int
	OutputCount  int
	MissingCount int
	DurationMS   int64
}

type diagnosticObserverKey struct{}

// WithDiagnosticObserver attaches a run-scoped diagnostic sink.
func WithDiagnosticObserver(ctx context.Context, callback func(DiagnosticObservation)) context.Context {
	if ctx == nil || callback == nil {
		return ctx
	}
	return context.WithValue(ctx, diagnosticObserverKey{}, callback)
}

func reportDiagnostic(ctx context.Context, observation DiagnosticObservation) {
	if ctx == nil {
		return
	}
	if callback, ok := ctx.Value(diagnosticObserverKey{}).(func(DiagnosticObservation)); ok && callback != nil {
		callback(observation)
	}
}

// diagnosticCode maps internal errors to stable, non-sensitive identifiers.
func diagnosticCode(err error) string {
	if err == nil {
		return "NONE"
	}
	if errors.Is(err, context.Canceled) {
		return "CONTEXT_CANCELED"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "CONTEXT_DEADLINE"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "invalid semantic evidence matcher json"):
		return "SEMANTIC_RESPONSE_INVALID"
	case strings.Contains(message, "semantic evidence matcher returned empty output"):
		return "SEMANTIC_RESPONSE_EMPTY"
	case strings.Contains(message, "generate semantic evidence relevance"):
		return "SEMANTIC_MODEL_ERROR"
	case strings.Contains(message, "semantic evidence research incomplete"):
		return "SEMANTIC_COVERAGE_INCOMPLETE"
	case strings.Contains(message, "validate semantic evidence") || strings.Contains(message, "validate retried semantic"):
		return "SEMANTIC_RESPONSE_INVALID"
	case strings.Contains(message, "persist evidence snapshots"):
		return "SNAPSHOT_PERSISTENCE_ERROR"
	case strings.Contains(message, "canonical"):
		return "CANONICALIZATION_ERROR"
	default:
		return "EVENT_ACQUISITION_ERROR"
	}
}
