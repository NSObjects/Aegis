package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func scheduleTestTask(kind ScheduleKind, id string) ScheduleTask {
	now := time.Now().UTC()
	task := ScheduleTask{ID: id, Name: "每日回踩扫描", Kind: kind, Enabled: true, CreatedAt: now, UpdatedAt: now}
	switch kind {
	case ScheduleKindScan:
		task.UniverseSource = "USER_SUPPLIED"
		task.Symbols = []string{"600519.SH"}
	case ScheduleKindResearch:
		task.Symbol = "600519.SH"
	}
	return task
}

func TestFileScheduleStoreTaskLifecycleAndValidation(t *testing.T) {
	store, err := NewFileScheduleStore(filepath.Join(t.TempDir(), "schedules"))
	if err != nil {
		t.Fatal(err)
	}
	task := scheduleTestTask(ScheduleKindScan, "0123456789abcdef")
	if err := store.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetTask(task.ID)
	if err != nil || got.Name != task.Name || !got.Enabled {
		t.Fatalf("task roundtrip: %+v %v", got, err)
	}
	// Kind is immutable; other fields may be updated.
	changed := task
	changed.Kind = ScheduleKindResearch
	if err := store.SaveTask(changed); err == nil {
		t.Fatal("kind mutation accepted")
	}
	changed = task
	changed.Enabled = false
	changed.UpdatedAt = time.Now().UTC()
	if err := store.SaveTask(changed); err != nil {
		t.Fatal(err)
	}
	tasks, err := store.ListTasks()
	if err != nil || len(tasks) != 1 || tasks[0].Enabled {
		t.Fatalf("list: %+v %v", tasks, err)
	}
	// Validation refusals.
	bad := scheduleTestTask(ScheduleKindScan, "bad-id")
	if err := store.SaveTask(bad); err == nil {
		t.Fatal("bad id accepted")
	}
	badSymbol := scheduleTestTask(ScheduleKindResearch, "0123456789abcdef")
	badSymbol.Symbol = "600519"
	if err := store.SaveTask(badSymbol); err == nil {
		t.Fatal("bad symbol accepted")
	}
	if err := store.SaveTask(scheduleTestTask(ScheduleKindResearch, "ffffffffffffffff")); err != nil {
		t.Fatal(err)
	}
	tasks, _ = store.ListTasks()
	if len(tasks) != 2 || tasks[0].ID > tasks[1].ID {
		t.Fatalf("tasks not sorted: %+v", tasks)
	}
}

func TestFileScheduleStoreRunLifecycleAndForwardOnly(t *testing.T) {
	store, err := NewFileScheduleStore(filepath.Join(t.TempDir(), "schedules"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	run := ScheduleRun{TaskID: "0123456789abcdef", TradeDate: "20261023", State: ScheduleRunning, StartedAt: now, UpdatedAt: now}
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRun(run.TaskID, run.TradeDate)
	if err != nil || got.State != ScheduleRunning {
		t.Fatalf("run roundtrip: %+v %v", got, err)
	}
	if _, err := store.GetRun(run.TaskID, "20261024"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing run err=%v", err)
	}
	run.State = ScheduleCompleted
	run.RefID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run.UpdatedAt = now.Add(time.Second)
	if err := store.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	// Terminal state must not move backwards.
	regression := run
	regression.State = ScheduleRunning
	if err := store.SaveRun(regression); err == nil {
		t.Fatal("backwards transition accepted")
	}
	// Completed without ref is invalid; failed requires a code.
	noRef := run
	noRef.RefID = ""
	if err := store.SaveRun(noRef); err == nil {
		t.Fatal("completed without ref accepted")
	}
	failed := ScheduleRun{TaskID: run.TaskID, TradeDate: "20261024", State: ScheduleFailed, StartedAt: now, UpdatedAt: now, FailureCode: "TRIGGER_FAILED: boom"}
	if err := store.SaveRun(failed); err != nil {
		t.Fatal(err)
	}
	noCode := failed
	noCode.FailureCode = ""
	if err := store.SaveRun(noCode); err == nil {
		t.Fatal("failed without code accepted")
	}
}

func TestFileScheduleReconcileInterrupted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "schedules")
	store, err := NewFileScheduleStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	running := ScheduleRun{TaskID: "0123456789abcdef", TradeDate: "20261023", State: ScheduleRunning, StartedAt: now, UpdatedAt: now}
	if err := store.SaveRun(running); err != nil {
		t.Fatal(err)
	}
	done := ScheduleRun{TaskID: "0123456789abcdef", TradeDate: "20261022", State: ScheduleCompleted, RefID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", StartedAt: now, UpdatedAt: now}
	if err := store.SaveRun(done); err != nil {
		t.Fatal(err)
	}
	// A new process over the same directory reconciles.
	reopened, err := NewFileScheduleStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := reopened.ReconcileInterrupted()
	if err != nil || changed != 1 {
		t.Fatalf("reconcile changed=%d err=%v", changed, err)
	}
	got, err := reopened.GetRun(running.TaskID, running.TradeDate)
	if err != nil || got.State != ScheduleInterrupted || got.FailureCode != "PROCESS_INTERRUPTED" {
		t.Fatalf("running not interrupted: %+v %v", got, err)
	}
	if terminal, err := reopened.GetRun(done.TaskID, done.TradeDate); err != nil || terminal.State != ScheduleCompleted {
		t.Fatalf("completed record disturbed: %+v %v", terminal, err)
	}
}

type calendarStub struct {
	open bool
	err  error
}

func (c calendarStub) IsTradingDay(context.Context, string) (bool, error) { return c.open, c.err }

type runnerStub struct {
	scanErr, researchErr                   error
	scanCalls, researchCalls, monitorCalls int
	monitorFn                              func(context.Context, ScheduleTask, string) error
}

func (r *runnerStub) RunScan(context.Context, ScheduleTask, string) (string, error) {
	r.scanCalls++
	if r.scanErr != nil {
		return "", r.scanErr
	}
	return "scan-ref-0000000000000000000000000000ffff", nil
}
func (r *runnerStub) RunResearch(context.Context, ScheduleTask, string) (string, error) {
	r.researchCalls++
	if r.researchErr != nil {
		return "", r.researchErr
	}
	return "run-ref-000000000000000000000000000000", nil
}
func (r *runnerStub) RunMonitor(ctx context.Context, task ScheduleTask, tradeDate string) (string, error) {
	r.monitorCalls++
	if r.monitorFn != nil {
		if err := r.monitorFn(ctx, task, tradeDate); err != nil {
			return "", err
		}
	}
	return "", nil
}

type trackerStub struct {
	state map[string]struct {
		terminal, failed bool
		code             string
		err              error
	}
}

func (t trackerStub) RefTerminal(_ context.Context, _ ScheduleTask, refID string) (bool, bool, string, error) {
	s, ok := t.state[refID]
	if !ok {
		return false, false, "", nil
	}
	return s.terminal, s.failed, s.code, s.err
}

func newSchedulerTest(t *testing.T, cal calendarStub, runner *runnerStub, tracker trackerStub, clock func() time.Time) (*Scheduler, *FileScheduleStore) {
	t.Helper()
	store, err := NewFileScheduleStore(filepath.Join(t.TempDir(), "schedules"))
	if err != nil {
		t.Fatal(err)
	}
	sch, err := NewScheduler(store, cal, runner, tracker, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	sch.now = clock
	return sch, store
}

func TestSchedulerTriggersOncePerTradingDay(t *testing.T) {
	runner := &runnerStub{}
	sch, store := newSchedulerTest(t, calendarStub{open: true}, runner, trackerStub{}, func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) })
	task := scheduleTestTask(ScheduleKindScan, "0123456789abcdef")
	if err := store.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	research := scheduleTestTask(ScheduleKindResearch, "ffffffffffffffff")
	if err := store.SaveTask(research); err != nil {
		t.Fatal(err)
	}
	// Two ticks on the same trading day trigger each task exactly once.
	sch.Tick(context.Background())
	sch.Tick(context.Background())
	if runner.scanCalls != 1 || runner.researchCalls != 1 {
		t.Fatalf("repeat trigger on same day: scan=%d research=%d", runner.scanCalls, runner.researchCalls)
	}
	got, err := store.GetRun(task.ID, "20261023")
	if err != nil || got.State != ScheduleRunning || got.RefID == "" {
		t.Fatalf("run record: %+v %v", got, err)
	}
	// Next trading day triggers again.
	sch.now = func() time.Time { return time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC) }
	sch.Tick(context.Background())
	if runner.scanCalls != 2 {
		t.Fatalf("new trading day not triggered: %d", runner.scanCalls)
	}
}

func TestSchedulerSkipsNonTradingDaysAndCalendarErrors(t *testing.T) {
	runner := &runnerStub{}
	sch, store := newSchedulerTest(t, calendarStub{open: false}, runner, trackerStub{}, func() time.Time { return time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC) })
	if err := store.SaveTask(scheduleTestTask(ScheduleKindResearch, "0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	sch.Tick(context.Background())
	if runner.researchCalls != 0 {
		t.Fatal("triggered on non-trading day")
	}
	// Calendar unavailable must skip loudly, never trigger.
	sch.calendar = calendarStub{err: errors.New("provider down")}
	sch.Tick(context.Background())
	if runner.researchCalls != 0 {
		t.Fatal("triggered on unverifiable calendar")
	}
	// Disabled tasks never trigger even on trading days.
	sch.calendar = calendarStub{open: true}
	task, _ := store.GetTask("0123456789abcdef")
	task.Enabled = false
	task.UpdatedAt = time.Now().UTC()
	if err := store.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	sch.Tick(context.Background())
	if runner.researchCalls != 0 {
		t.Fatal("disabled task triggered")
	}
}

func TestSchedulerTriggerFailureRecordsAndConsumesDay(t *testing.T) {
	runner := &runnerStub{researchErr: errors.New("capacity reached")}
	sch, store := newSchedulerTest(t, calendarStub{open: true}, runner, trackerStub{}, func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) })
	if err := store.SaveTask(scheduleTestTask(ScheduleKindResearch, "0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	sch.Tick(context.Background())
	if runner.researchCalls != 1 {
		t.Fatal("trigger not attempted")
	}
	got, err := store.GetRun("0123456789abcdef", "20261023")
	if err != nil || got.State != ScheduleFailed || got.FailureCode == "" {
		t.Fatalf("failure not recorded: %+v %v", got, err)
	}
	// Retried ticks do not re-trigger: the day is consumed by the record.
	sch.Tick(context.Background())
	if runner.researchCalls != 1 {
		t.Fatal("failed day re-triggered")
	}
}

func TestSchedulerAdvancesRunningToTerminal(t *testing.T) {
	runner := &runnerStub{}
	tracker := trackerStub{state: map[string]struct {
		terminal, failed bool
		code             string
		err              error
	}{}}
	sch, store := newSchedulerTest(t, calendarStub{open: true}, runner, tracker, func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) })
	if err := store.SaveTask(scheduleTestTask(ScheduleKindResearch, "0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	sch.Tick(context.Background())
	refID := "run-ref-000000000000000000000000000000"
	// Engine still running: record stays RUNNING.
	sch.Tick(context.Background())
	got, _ := store.GetRun("0123456789abcdef", "20261023")
	if got.State != ScheduleRunning {
		t.Fatalf("premature terminal: %+v", got)
	}
	tracker.state[refID] = struct {
		terminal, failed bool
		code             string
		err              error
	}{terminal: true, failed: true, code: "RESEARCH_FAILED"}
	sch.Tick(context.Background())
	got, _ = store.GetRun("0123456789abcdef", "20261023")
	if got.State != ScheduleFailed || got.FailureCode != "RESEARCH_FAILED" {
		t.Fatalf("failure not advanced: %+v", got)
	}
	// A tracker error leaves the record RUNNING for the next tick.
	if err := store.SaveTask(scheduleTestTask(ScheduleKindScan, "ffffffffffffffff")); err != nil {
		t.Fatal(err)
	}
	sch.Tick(context.Background())
	tracker.state["scan-ref-0000000000000000000000000000ffff"] = struct {
		terminal, failed bool
		code             string
		err              error
	}{terminal: false, failed: false, err: errors.New("poll failed")}
	sch.Tick(context.Background())
	got, _ = store.GetRun("ffffffffffffffff", "20261023")
	if got.State != ScheduleRunning {
		t.Fatalf("poll error advanced state: %+v", got)
	}
	tracker.state["scan-ref-0000000000000000000000000000ffff"] = struct {
		terminal, failed bool
		code             string
		err              error
	}{terminal: true, failed: false}
	sch.Tick(context.Background())
	got, _ = store.GetRun("ffffffffffffffff", "20261023")
	if got.State != ScheduleCompleted {
		t.Fatalf("completion not advanced: %+v", got)
	}
}

func TestSchedulerRestartDoesNotRepeatPaidTrigger(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "schedules")
	store, err := NewFileScheduleStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	task := scheduleTestTask(ScheduleKindResearch, "0123456789abcdef")
	if err := store.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	runner := &runnerStub{}
	sch, _ := newSchedulerTest(t, calendarStub{open: true}, runner, trackerStub{}, func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) })
	// Replace the store with one sharing the directory so the scheduler and the
	// "previous process" read the same files.
	sch.store = store
	sch.Tick(context.Background())
	if runner.researchCalls != 1 {
		t.Fatal("first trigger missing")
	}
	// Simulated crash: a new scheduler over the same directory reconciles, then
	// ticks on the same trading day. The paid research must not re-trigger.
	reopened, err := NewFileScheduleStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := reopened.ReconcileInterrupted(); err != nil || changed != 1 {
		t.Fatalf("reconcile %d %v", changed, err)
	}
	runner2 := &runnerStub{}
	sch2, err := NewScheduler(reopened, calendarStub{open: true}, runner2, trackerStub{}, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	sch2.now = func() time.Time { return time.Date(2026, 10, 23, 10, 5, 0, 0, time.UTC) }
	sch2.Tick(context.Background())
	if runner2.researchCalls != 0 {
		t.Fatal("interrupted research re-triggered on restart")
	}
	got, err := reopened.GetRun(task.ID, "20261023")
	if err != nil || got.State != ScheduleInterrupted {
		t.Fatalf("record after restart: %+v %v", got, err)
	}
	// The next trading day is a fresh day and does trigger.
	sch2.now = func() time.Time { return time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC) }
	sch2.Tick(context.Background())
	if runner2.researchCalls != 1 {
		t.Fatal("next trading day not triggered after interruption")
	}
}

func TestSchedulerRunsMonitorPassOnTradingDays(t *testing.T) {
	monitorStore, err := NewFileMonitorStore(filepath.Join(t.TempDir(), "monitor"))
	if err != nil {
		t.Fatal(err)
	}
	source := &monitorSourceStub{obs: monitorBarsWithClose(100, 1000), obsOK: true}
	monitor, err := NewMonitor(monitorStore, source)
	if err != nil {
		t.Fatal(err)
	}
	runner := &runnerStub{monitorFn: func(_ context.Context, _ ScheduleTask, tradeDate string) error {
		return monitor.Tick(context.Background(), tradeDate)
	}}
	tracker := trackerStub{}
	sch, store := newSchedulerTest(t, calendarStub{open: true}, runner, tracker, func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) })
	task := ScheduleTask{ID: "0123456789abcdef", Name: "观察池监控", Kind: ScheduleKindMonitor, Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := store.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	rule := monitorTestRule(MonitorPriceAbove, "ffffffffffffffff", 50)
	if err := monitorStore.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	sch.Tick(context.Background())
	if runner.monitorCalls != 1 {
		t.Fatalf("monitor pass not triggered: %d", runner.monitorCalls)
	}
	// The synchronous monitor pass completes on the next tick.
	sch.Tick(context.Background())
	got, err := store.GetRun(task.ID, "20261023")
	if err != nil || got.State != ScheduleCompleted {
		t.Fatalf("monitor run not completed: %+v %v", got, err)
	}
	// The monitor actually evaluated the rule (close 100 > 50 → FIRED).
	event, err := monitorStore.GetEvent(rule.ID, "20261023")
	if err != nil || event.Outcome != MonitorFired {
		t.Fatalf("rule not evaluated: %+v %v", event, err)
	}
	// Repeat ticks consume the day: no second pass.
	sch.Tick(context.Background())
	if runner.monitorCalls != 1 {
		t.Fatalf("day re-triggered: %d", runner.monitorCalls)
	}
}
