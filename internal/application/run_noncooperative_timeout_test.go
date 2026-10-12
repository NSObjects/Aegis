package application

import (
 "context"
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
 close(ex.release)
 if readErr != nil { t.Fatal(readErr) }
 if observed.State != RunFailed || observed.FailureCode != FailureTimeout {
  t.Fatalf("deadline did not finalize non-cooperative run: state=%s code=%s", observed.State, observed.FailureCode)
 }
}
