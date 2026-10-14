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
