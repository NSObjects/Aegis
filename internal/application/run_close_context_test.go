package application

import (
 "context"
 "errors"
 "sync"
 "testing"
 "time"
)

func TestCloseContextTimesOutWithoutAbandoningExecutor(t *testing.T) {
 store := newStore(t)
 ex := &controlledRunExecutor{started: make(chan struct{}, 1), release: make(chan struct{}), ignoreCancel: true}
 var once sync.Once
 release := func() { once.Do(func() { close(ex.release) }) }
 defer release()
 manager := newMgr(t, ex, store, 1, 1)
 run, err := manager.Submit(context.Background(), runFixture("600519.SH"))
 if err != nil { t.Fatal(err) }
 select {
 case <-ex.started:
 case <-time.After(2*time.Second): t.Fatal("executor did not start")
 }
 ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
 defer cancel()
 if err := manager.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
  t.Fatalf("stuck executor must not be claimed drained: %v", err)
 }
 // Closing cancels the run; its watchdog persists the terminal state
 // asynchronously. Poll while keeping the non-cooperative executor blocked.
 var observed RunRecord
 terminalDeadline := time.After(2*time.Second)
 for {
  observed, err = manager.Get(context.Background(), run.ID)
  if err != nil { t.Fatal(err) }
  if observed.State == RunCancelled && observed.FinishedAt != nil { break }
  select {
  case <-terminalDeadline: t.Fatalf("cancelled outcome was not durably recorded: %+v", observed)
  case <-time.After(10*time.Millisecond):
  }
 }
 if len(observed.Artifact) != 0 { t.Fatalf("cancelled Run must not expose artifact: %+v", observed) }
 if _, err := manager.Submit(context.Background(), runFixture("000001.SZ")); !errors.Is(err, ErrRunManagerClosed) {
  t.Fatalf("bounded close must reject future work: %v", err)
 }
 // A second close may wait for the same worker; releasing the executor
 // actually drains the slot, without manufacturing a successful artifact.
 release()
 wait, stop := context.WithTimeout(context.Background(), 2*time.Second)
 defer stop()
 if err := manager.CloseContext(wait); err != nil { t.Fatalf("released executor did not drain: %v", err) }
 final, err := manager.Get(context.Background(), run.ID)
 if err != nil { t.Fatal(err) }
 if final.State != RunCancelled || len(final.Artifact) != 0 { t.Fatalf("late result overwrote cancellation: %+v", final) }
}
