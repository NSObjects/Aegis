package research

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestProgressEventsPreserveExistingWriterAndEmitTypedMetadata(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	var events []ProgressEvent
	ctx := WithProgressEvents(WithProgress(context.Background(), &buf), func(e ProgressEvent) { mu.Lock(); events = append(events, e); mu.Unlock() })
	done := startProgress(ctx, "Market")
	done(nil)
	if len(events) != 2 || events[0].Status != "START" || events[1].Status != "DONE" || events[0].At.IsZero() || events[1].At.IsZero() {
		t.Fatalf("wrong event sequence: %+v", events)
	}
	if !strings.Contains(buf.String(), "stage=Market status=START") || !strings.Contains(buf.String(), "stage=Market status=DONE duration=") {
		t.Fatalf("legacy progress changed: %s", buf.String())
	}
	fail := startProgress(ctx, "Risk")
	e := errors.New("SECRET_PRIVATE_PROVIDER_ERROR")
	fail(&e)
	if events[len(events)-1].Status != "FAIL" || strings.Contains(buf.String(), e.Error()) {
		t.Fatal("raw provider error leaked")
	}
}
func TestProgressCallbackWorksWithoutTextWriter(t *testing.T) {
	called := 0
	ctx := WithProgressEvents(context.Background(), func(e ProgressEvent) { called++ })
	finish := startProgress(ctx, "Portfolio")
	finish(nil)
	if called != 2 {
		t.Fatalf("expected start and finish, got %d", called)
	}
}
