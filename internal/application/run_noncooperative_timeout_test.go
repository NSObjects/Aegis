package application

import (
 "context"
 "errors"
 "sync"
 "testing"
 "time"
)

// Regression for a provider which ignores context cancellation.
// Always release the provider before returning so this test cannot hang cleanup.
func TestNonCooperativeExecutorMustNotLeaveRunRunningAfterDeadline(t *testing.T) {
 store := newStore(t)
 ex := &controlledRunExecutor{started: make(chan struct{}, 1), release: make(chan struct{}), ignoreCancel: true}
 var once sync.Once
 release := func(){once.Do(func(){close(ex.release)})}
 defer release()
 manager := newMgr(t, ex, store, 1, 1)
 if err := manager.SetRunTimeout(60*time.Millisecond); err != nil { t.Fatal(err) }
 run, err := manager.Submit(context.Background(), runFixture("600519.SH"))
 if err != nil { t.Fatal(err) }
 select {
 case <-ex.started:
 case <-time.After(time.Second): release(); t.Fatal("executor did not start")
 }
 // The watchdog finalizes asynchronously when ctx expires. Poll the durable
 // state instead of assuming a 150ms sleep schedules the watchdog on loaded CI.
 // Keep the provider blocked throughout to verify capacity is not released.
 var observed RunRecord
 deadline := time.After(2*time.Second)
 for {
  var readErr error
  observed, readErr = manager.Get(context.Background(), run.ID)
  if readErr != nil { release(); t.Fatal(readErr) }
  if observed.State == RunFailed && observed.FailureCode == FailureTimeout && observed.FinishedAt != nil { break }
  select {
  case <-deadline: release(); t.Fatalf("deadline watchdog did not persist terminal state: %+v", observed)
  case <-time.After(10*time.Millisecond):
  }
 }
 // A timed-out but still executing provider must continue occupying capacity.
 _, capacityErr := manager.Submit(context.Background(), runFixture("000001.SZ"))
 release()
 if !errors.Is(capacityErr, ErrRunCapacity) { t.Fatalf("blocked executor prematurely released capacity: %v", capacityErr) }
 if observed.FinishedAt == nil || observed.FailureCategory != "TIMEOUT" {
  t.Fatalf("timeout must have timestamp and safe category: %+v", observed)
 }
 if observed.State != RunFailed || observed.FailureCode != FailureTimeout {
  t.Fatalf("deadline did not finalize non-cooperative run: state=%s code=%s", observed.State, observed.FailureCode)
 }
 manager.wg.Wait()
 final,err:=manager.Get(context.Background(),run.ID)
 if err!=nil { t.Fatal(err) }
 if final.State != RunFailed || final.FailureCode != FailureTimeout || len(final.Artifact)!=0 {
  t.Fatalf("late successful executor overwrote timeout: %+v",final)
 }
}


// A bounded shutdown reports pending ownership without pretending to kill a
// provider that ignored cancellation. An eventual second wait drains it.
func TestRunManagerCloseContextPreservesBlockedExecutorOwnership(t *testing.T) {
 store := newStore(t)
 ex := &controlledRunExecutor{started: make(chan struct{}, 1), release: make(chan struct{}), ignoreCancel: true}
 var once sync.Once
 release := func(){once.Do(func(){close(ex.release)})}
 defer release()
 manager := newMgr(t, ex, store, 1, 1)
 run, err := manager.Submit(context.Background(), runFixture("600519.SH"))
 if err != nil { t.Fatal(err) }
 select {
 case <-ex.started:
 case <-time.After(time.Second): t.Fatal("executor did not start")
 }
 ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
 defer cancel()
 if err := manager.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
  release()
  t.Fatalf("blocked executor must not be treated as drained: %v", err)
 }
 if len(manager.slots) != 1 { release(); t.Fatal("uncooperative executor prematurely released slot") }
 observed, err := manager.Get(context.Background(), run.ID)
 if err != nil { release(); t.Fatal(err) }
 if observed.State != RunCancelled || observed.FinishedAt == nil || len(observed.Artifact) != 0 {
  release()
  t.Fatalf("shutdown must durably cancel without allowing late artifacts: %+v", observed)
 }
 release()
 doneCtx, doneCancel := context.WithTimeout(context.Background(), time.Second)
 defer doneCancel()
 if err := manager.CloseContext(doneCtx); err != nil {
  t.Fatalf("released provider did not drain: %v", err)
 }
 final, err := manager.Get(context.Background(), run.ID)
 if err != nil { t.Fatal(err) }
 if final.State != RunCancelled || len(final.Artifact) != 0 { t.Fatalf("late result changed terminal Run: %+v", final) }
}
