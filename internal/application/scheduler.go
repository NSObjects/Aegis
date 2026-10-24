package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"Aegis/internal/capability/technical"
)

// ScheduleActionRunner triggers the existing engines. Runners receive the
// persisted task; refID links the execution record to the engine's own
// durable object (scan_id or run_id).
type ScheduleActionRunner interface {
	RunScan(ctx context.Context, task ScheduleTask, tradeDate string) (string, error)
	RunResearch(ctx context.Context, task ScheduleTask, tradeDate string) (string, error)
	RunMonitor(ctx context.Context, task ScheduleTask, tradeDate string) (string, error)
}

// ScheduleRefTracker reports whether a triggered engine object reached a
// terminal state, so execution records can advance past RUNNING.
type ScheduleRefTracker interface {
	RefTerminal(ctx context.Context, task ScheduleTask, refID string) (terminal bool, failed bool, failureCode string, err error)
}

// Scheduler is a single-process daily trigger over persisted tasks. Its two
// hard guarantees: one execution record per (task, trading day) — a day is
// never triggered twice, whatever happens — and no automatic re-triggering of
// interrupted or failed paid work.
type Scheduler struct {
	store    *FileScheduleStore
	calendar technical.TradingCalendar
	runner   ScheduleActionRunner
	tracker  ScheduleRefTracker
	location *time.Location
	now      func() time.Time

	mu      sync.Mutex
	running bool
	stop    chan struct{}
	done    chan struct{}
}

func NewScheduler(store *FileScheduleStore, calendar technical.TradingCalendar, runner ScheduleActionRunner, tracker ScheduleRefTracker, location *time.Location) (*Scheduler, error) {
	if store == nil || calendar == nil || runner == nil || tracker == nil {
		return nil, errors.New("scheduler requires store, calendar, runner and tracker")
	}
	if location == nil {
		return nil, errors.New("scheduler timezone required")
	}
	return &Scheduler{store: store, calendar: calendar, runner: runner, tracker: tracker, location: location, now: func() time.Time { return time.Now().UTC() }, stop: make(chan struct{}), done: make(chan struct{})}, nil
}

// ReconcileInterrupted must run once at startup before the loop starts:
// records left RUNNING by a previous process become INTERRUPTED and their
// trading day stays consumed.
func (s *Scheduler) ReconcileInterrupted() (int, error) { return s.store.ReconcileInterrupted() }

// Start launches the background loop; Stop ends it. A scheduler runs once
// per process; calling Start twice is a programming error.
func (s *Scheduler) Start(interval time.Duration) error {
	if interval <= 0 {
		return errors.New("scheduler interval must be positive")
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("scheduler already running")
	}
	s.running = true
	s.mu.Unlock()
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		s.tickOnce() // fire immediately so a mid-day start still triggers due work
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.tickOnce()
			}
		}
	}()
	return nil
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	close(s.stop)
	s.running = false
	s.mu.Unlock()
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
	}
}

// tickOnce is the whole scheduling decision: trading day check, then
// trigger missing records, then advance RUNNING records.
func (s *Scheduler) tickOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s.tickContext(ctx)
}

func (s *Scheduler) tickContext(ctx context.Context) {
	today := s.now().In(s.location).Format("20060102")
	trading, err := s.calendar.IsTradingDay(ctx, today)
	if err != nil {
		// Calendar unavailable: skip the cycle loudly. Never trigger paid work
		// on an unverifiable day.
		log.Printf("AEGIS_SCHEDULER_SKIP reason=CALENDAR_UNAVAILABLE err=%v", err)
		return
	}
	if !trading {
		return
	}
	tasks, err := s.store.ListTasks()
	if err != nil {
		log.Printf("AEGIS_SCHEDULER_SKIP reason=TASK_LIST_UNAVAILABLE err=%v", err)
		return
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		if !task.Enabled {
			continue
		}
		if err := s.processTask(ctx, task, today); err != nil {
			log.Printf("AEGIS_SCHEDULER_TASK_ERROR task=%s err=%v", task.ID, err)
		}
	}
}

func (s *Scheduler) processTask(ctx context.Context, task ScheduleTask, tradeDate string) error {
	existing, err := s.store.GetRun(task.ID, tradeDate)
	if err == nil {
		if existing.State != ScheduleRunning {
			return nil
		}
		return s.advanceRunning(ctx, task, existing)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read run record: %w", err)
	}
	// No record for this trading day: trigger exactly once. The record is
	// written before the engine call so a crash mid-trigger still consumes the
	// day instead of re-triggering paid work on restart.
	now := s.now().UTC()
	record := ScheduleRun{TaskID: task.ID, TradeDate: tradeDate, State: ScheduleRunning, StartedAt: now, UpdatedAt: now}
	if err := s.store.SaveRun(record); err != nil {
		return fmt.Errorf("reserve trading day: %w", err)
	}
	var refID string
	var runErr error
	switch task.Kind {
	case ScheduleKindScan:
		refID, runErr = s.runner.RunScan(ctx, task, tradeDate)
	case ScheduleKindResearch:
		refID, runErr = s.runner.RunResearch(ctx, task, tradeDate)
	case ScheduleKindMonitor:
		refID, runErr = s.runner.RunMonitor(ctx, task, tradeDate)
	default:
		runErr = errors.New("unknown schedule kind")
	}
	if runErr != nil {
		record.State = ScheduleFailed
		record.FailureCode = scheduleFailureCode(runErr)
		record.UpdatedAt = s.now().UTC()
		if saveErr := s.store.SaveRun(record); saveErr != nil {
			return saveErr
		}
		return nil
	}
	record.RefID = refID
	record.UpdatedAt = s.now().UTC()
	return s.store.SaveRun(record)
}

func scheduleFailureCode(err error) string {
	msg := err.Error()
	if len(msg) > 64 {
		msg = msg[:64]
	}
	return "TRIGGER_FAILED: " + msg
}

// advanceRunning polls the engine object and moves the record to its
// terminal state. Poll errors keep the record RUNNING for the next tick.
func (s *Scheduler) advanceRunning(ctx context.Context, task ScheduleTask, record ScheduleRun) error {
	if task.Kind == ScheduleKindMonitor {
		// The monitor pass is synchronous: a RUNNING record without a ref has
		// finished by construction. The sentinel ref satisfies the completed
		// record contract without inventing an engine object.
		record.State = ScheduleCompleted
		record.RefID = "MONITOR_PASS"
		record.UpdatedAt = s.now().UTC()
		return s.store.SaveRun(record)
	}
	if record.RefID == "" {
		// A RUNNING record without a ref means the process died between
		// reserving the day and receiving an engine object; reconciliation at
		// startup covers this, but if it is still RUNNING the trigger path must
		// have failed to persist. Mark failed rather than loop forever.
		record.State = ScheduleFailed
		record.FailureCode = "TRIGGER_REF_MISSING"
		record.UpdatedAt = s.now().UTC()
		return s.store.SaveRun(record)
	}
	terminal, failed, code, err := s.tracker.RefTerminal(ctx, task, record.RefID)
	if err != nil {
		return nil
	}
	if !terminal {
		return nil
	}
	if failed {
		record.State = ScheduleFailed
		record.FailureCode = code
	} else {
		record.State = ScheduleCompleted
	}
	record.UpdatedAt = s.now().UTC()
	return s.store.SaveRun(record)
}

// Tick exposes one synchronous scheduling pass for tests and CLI inspection.
func (s *Scheduler) Tick(ctx context.Context) { s.tickContext(ctx) }
