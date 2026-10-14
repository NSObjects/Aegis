package application

import (
 "context"
 "errors"
 "sync"
 "testing"
 "time"
)

// Close must retain ownership of a non-cooperative executor while the
// independent watchdog makes its cancellation terminal state durable.
func TestCloseWaitsForNonCooperativeExecutorWhileCancellationPersists(t *testing.T) {
 store := newStore(t)
 ex := &controlledRunExecutor{started: make(chan struct{}, 1), release: make(chan struct{}), ignoreCancel: true}
 var once sync.Once
 release := func() { once.Do(func() { close(ex.release) }) }
 defer release()
 manager := newMgr(t, ex, store, 1, 1)
 // Keep the normal run deadline (20m): this test exercises explicit
 // shutdown cancellation, not the competing research TIMEOUT path.
 run, err := manager.Submit(context.Background(), runFixture("600519.SH"))
 if err != nil { t.Fatal(err) }
 select {
 case <-ex.started:
 case <-time.After(2*time.Second): t.Fatal("executor did not start")
 }
 closed := make(chan struct{})
 go func() { manager.Close(); close(closed) }()
 select {
 case <-closed: t.Fatal("Close returned while executor still owned the slot")
 case <-time.After(100*time.Millisecond):
 }
 deadline := time.After(2*time.Second)
 for {
  observed, err := manager.Get(context.Background(), run.ID)
  if err != nil { t.Fatal(err) }
  if observed.State == RunCancelled && observed.FinishedAt != nil {
   if len(observed.Artifact) != 0 { t.Fatal("cancelled Run exposed an artifact") }
   break
  }
  select {
  case <-deadline: t.Fatalf("terminal cancellation not persisted: state=%s code=%s", observed.State, observed.FailureCode)
  case <-time.After(10*time.Millisecond):
  }
 }
 if _, err := manager.Submit(context.Background(), runFixture("000001.SZ")); !errors.Is(err, ErrRunManagerClosed) {
  t.Fatalf("Close must reject new work: %v", err)
 }
 release()
 select {
 case <-closed:
 case <-time.After(2*time.Second): t.Fatal("Close did not drain after executor was released")
 }
}
