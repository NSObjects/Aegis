package application

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "testing"
 "time"
)

// TestCloseContextSubprocess verifies that a bounded shutdown can return in an
// actual OS process even while a deliberately non-cooperative executor remains
// blocked. No paid providers or test-only production hooks are involved.
func TestCloseContextSubprocess(t *testing.T) {
 if os.Getenv("AEGIS_TEST_STUCK_EXECUTOR_HELPER") == "1" {
  dir := os.Getenv("AEGIS_TEST_STUCK_EXECUTOR_STORE")
  store, err := NewFileRunStore(dir)
  if err != nil { t.Fatal(err) }
  ex := &controlledRunExecutor{started: make(chan struct{}, 1), release: make(chan struct{}), ignoreCancel: true}
  manager, err := NewRunManager(ex, store, 1, 1)
  if err != nil { t.Fatal(err) }
  run, err := manager.Submit(context.Background(), runFixture("600519.SH"))
  if err != nil { t.Fatal(err) }
  select {
  case <-ex.started:
  case <-time.After(2*time.Second): t.Fatal("executor did not start")
  }
  ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
  defer cancel()
  if err := manager.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
   t.Fatalf("still-running executor must not be reported drained: %v", err)
  }
  // A bounded close does not promise the watchdog has already persisted its
  // terminal event. Check it before the test process exits.
  deadline := time.After(2*time.Second)
  for {
   observed, err := manager.Get(context.Background(), run.ID)
   if err != nil { t.Fatal(err) }
   if observed.State == RunCancelled && observed.FinishedAt != nil && len(observed.Artifact) == 0 { break }
   select {
   case <-deadline: t.Fatalf("shutdown did not durably cancel blocked executor: state=%s", observed.State)
   case <-time.After(10*time.Millisecond):
   }
  }
  // Intentionally do not release the executor: Go test process exit must
  // not await a goroutine whose external provider refuses cancellation.
  return
 }
 dir := filepath.Join(t.TempDir(), "runs")
 binary, err := os.Executable()
 if err != nil { t.Fatal(err) }
 ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
 defer cancel()
 cmd := exec.CommandContext(ctx, binary, "-test.run=^TestCloseContextSubprocess$", "-test.count=1")
 cmd.Env = append(os.Environ(), "AEGIS_TEST_STUCK_EXECUTOR_HELPER=1", "AEGIS_TEST_STUCK_EXECUTOR_STORE="+dir)
 output, err := cmd.CombinedOutput()
 if ctx.Err() != nil { t.Fatalf("non-cooperative shutdown exceeded process deadline: %v; output: %s", ctx.Err(), output) }
 if err != nil { t.Fatalf("non-cooperative subprocess failed: %v; output: %s", err, output) }
 store, err := NewFileRunStore(dir)
 if err != nil { t.Fatal(err) }
 runs, err := store.List(context.Background())
 if err != nil { t.Fatal(err) }
 if len(runs) != 1 { t.Fatalf("expected one persisted run, got %d", len(runs)) }
 if runs[0].State != RunCancelled || runs[0].FinishedAt == nil || len(runs[0].Artifact) != 0 {
  t.Fatalf("non-cooperative subprocess did not persist safe terminal state: %+v", runs[0])
 }
}
