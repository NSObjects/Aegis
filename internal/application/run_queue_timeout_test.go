package application

import (
 "context"
 "testing"
 "time"
)

// A queued Run must expire without invoking paid models even when an
// uncooperative earlier executor still holds the only execution slot.
func TestQueuedRunDeadlineNeverStartsExecutor(t *testing.T) {
 store := newStore(t)
 executor := &controlledRunExecutor{
  started: make(chan struct{}, 2),
  release: make(chan struct{}),
  ignoreCancel: true,
 }
 manager := newMgr(t, executor, store, 1, 2)
 if err := manager.SetRunTimeout(120 * time.Millisecond); err != nil { t.Fatal(err) }
 first, err := manager.Submit(context.Background(), runFixture("600519.SH"))
 if err != nil { t.Fatal(err) }
 select {
 case <-executor.started:
 case <-time.After(time.Second): t.Fatal("first executor did not start")
 }
 second, err := manager.Submit(context.Background(), runFixture("000001.SZ"))
 if err != nil { t.Fatal(err) }
 // Second attempt must time out in the queue; the first executor deliberately
 // ignores context and cannot release its slot until the test explicitly does.
 queued := awaitRun(t, manager, second.ID, RunFailed)
 if queued.FailureCode != FailureTimeout || queued.FailureCategory != "TIMEOUT" {
  t.Fatalf("queued timeout not persisted: %+v", queued)
 }
 if got := executor.count(); got != 1 {
  t.Fatalf("queued Run invoked executor despite timeout: calls=%d", got)
 }
 close(executor.release)
 _ = awaitRun(t, manager, first.ID, RunFailed)
 reopened, err := NewFileRunStore(store.dir)
 if err != nil { t.Fatal(err) }
 persisted, err := reopened.Get(context.Background(), second.ID)
 if err != nil { t.Fatal(err) }
 if persisted.FailureCode != FailureTimeout || persisted.StartedAt != nil {
  t.Fatalf("queued timeout should not claim workflow start: %+v", persisted)
 }
}

func TestManualCancellationStillDistinctFromTimeout(t *testing.T) {
 store := newStore(t)
 executor := &controlledRunExecutor{started: make(chan struct{}, 1), release: make(chan struct{})}
 manager := newMgr(t, executor, store, 1, 2)
 if err := manager.SetRunTimeout(2*time.Second); err != nil { t.Fatal(err) }
 run, err := manager.Submit(context.Background(), runFixture("600519.SH"))
 if err != nil { t.Fatal(err) }
 select {
 case <-executor.started:
 case <-time.After(time.Second): t.Fatal("executor did not start")
 }
 if err := manager.Cancel(context.Background(), run.ID); err != nil { t.Fatal(err) }
 record := awaitRun(t, manager, run.ID, RunCancelled)
 if record.FailureCode == FailureTimeout || record.FailureCategory == "TIMEOUT" {
  t.Fatalf("manual cancel was treated as timeout: %+v", record)
 }
}
