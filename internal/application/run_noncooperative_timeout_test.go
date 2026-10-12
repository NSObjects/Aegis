package application

import (
 "context"
 "errors"
 "testing"
 "time"
)

// Regression for a provider which ignores context cancellation.
// Always release the provider before returning so this test cannot hang cleanup.
func TestNonCooperativeExecutorMustNotLeaveRunRunningAfterDeadline(t *testing.T) {
 store := newStore(t)
 ex := &controlledRunExecutor{started: make(chan struct{}, 1), release: make(chan struct{}), ignoreCancel: true}
 manager := newMgr(t, ex, store, 1, 1)
 if err := manager.SetRunTimeout(60*time.Millisecond); err != nil { t.Fatal(err) }
 run, err := manager.Submit(context.Background(), runFixture("600519.SH"))
 if err != nil { t.Fatal(err) }
 select {
 case <-ex.started:
 case <-time.After(time.Second): close(ex.release); t.Fatal("executor did not start")
 }
 // Inspect while the uncooperative provider is still running. A deadline
 // should be externally visible without interpreting tool START as success.
 time.Sleep(150*time.Millisecond)
 observed, readErr := manager.Get(context.Background(), run.ID)
 if readErr != nil { close(ex.release); t.Fatal(readErr) }
 // A timed-out but still executing provider must continue occupying capacity.
 _, capacityErr := manager.Submit(context.Background(), runFixture("000001.SZ"))
 close(ex.release)
 if !errors.Is(capacityErr, ErrRunCapacity) { t.Fatalf("blocked executor prematurely released capacity: %v", capacityErr) }
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
