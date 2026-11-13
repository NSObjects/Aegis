package application

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type triggerRunsStub struct {
	submitErr error
	submitted []ResearchRequest
	records   map[string]RunRecord
}

func (t *triggerRunsStub) Submit(_ context.Context, request ResearchRequest) (RunRecord, error) {
	if t.submitErr != nil {
		return RunRecord{}, t.submitErr
	}
	t.submitted = append(t.submitted, request)
	rec := RunRecord{ID: "runref" + string(rune('a'+len(t.submitted))), Mode: request.Mode, Symbol: request.Input.Symbol, State: RunRunning, CreatedAt: time.Now().UTC()}
	if t.records == nil {
		t.records = map[string]RunRecord{}
	}
	t.records[rec.ID] = rec
	return rec, nil
}
func (t *triggerRunsStub) Get(_ context.Context, id string) (RunRecord, error) {
	rec, ok := t.records[id]
	if !ok {
		return RunRecord{}, ErrRunNotFound
	}
	return rec, nil
}

func firedEvent(kind MonitorKind, ruleID, date string) MonitorEvent {
	return MonitorEvent{RuleID: ruleID, Symbol: "600519.SH", Kind: kind, TradeDate: date, Outcome: MonitorFired, Detail: "close=1499", CreatedAt: time.Now().UTC()}
}

func TestTriggerRouterGradesRoutesDeterministically(t *testing.T) {
	store, err := NewFileTriggerStore(filepath.Join(t.TempDir(), "triggers"))
	if err != nil {
		t.Fatal(err)
	}
	runs := &triggerRunsStub{}
	router, err := NewTriggerRouter(store, runs, ModeDecision, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Price break → context refresh, no engine run, completes immediately.
	ctx := context.Background()
	rec, err := router.Handle(ctx, firedEvent(MonitorPriceBelow, "0123456789abcdef", "20261023"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Route != RouteContextRefresh || rec.State != ScheduleCompleted || rec.RunID != "" || rec.ObservationDigest == "" {
		t.Fatalf("context refresh wrong: %+v", rec)
	}
	if len(runs.submitted) != 0 {
		t.Fatal("context refresh submitted a paid run")
	}
	// Announcement → thesis review starts the existing research engine once.
	annRec, err := router.Handle(ctx, firedEvent(MonitorAnnouncement, "ffffffffffffffff", "20261023"))
	if err != nil {
		t.Fatal(err)
	}
	if annRec.Route != RouteThesisReview || annRec.State != ScheduleRunning || annRec.RunID == "" {
		t.Fatalf("thesis review wrong: %+v", annRec)
	}
	if len(runs.submitted) != 1 {
		t.Fatal("thesis review did not submit")
	}
	if key := runs.submitted[0].IdempotencyKey; key != "trig-"+annRec.ID {
		t.Fatalf("idempotency key %s", key)
	}
	// Volume spike → context refresh like price.
	volRec, err := router.Handle(ctx, firedEvent(MonitorVolumeSpike, "abcdef0123456789", "20261023"))
	if err != nil {
		t.Fatal(err)
	}
	if volRec.Route != RouteContextRefresh {
		t.Fatalf("volume route: %+v", volRec)
	}
	// Non-FIRED events never route.
	if _, err := router.Handle(ctx, MonitorEvent{RuleID: "0123456789abcdef", Symbol: "600519.SH", Kind: MonitorPriceBelow, TradeDate: "20261024", Outcome: MonitorQuiet, CreatedAt: time.Now().UTC()}); err == nil {
		t.Fatal("quiet event routed")
	}
}

func TestTriggerRouterIdempotentPerRuleDay(t *testing.T) {
	store, err := NewFileTriggerStore(filepath.Join(t.TempDir(), "triggers"))
	if err != nil {
		t.Fatal(err)
	}
	runs := &triggerRunsStub{}
	router, _ := NewTriggerRouter(store, runs, ModeDecision, time.Hour)
	ctx := context.Background()
	first, err := router.Handle(ctx, firedEvent(MonitorAnnouncement, "0123456789abcdef", "20261023"))
	if err != nil {
		t.Fatal(err)
	}
	// Re-handling the same (rule, day) returns the same record, no second run.
	second, err := router.Handle(ctx, firedEvent(MonitorAnnouncement, "0123456789abcdef", "20261023"))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(runs.submitted) != 1 {
		t.Fatalf("duplicate handling: %+v %+v runs=%d", first, second, len(runs.submitted))
	}
	// A different trade date is a new trigger.
	third, err := router.Handle(ctx, firedEvent(MonitorAnnouncement, "0123456789abcdef", "20261024"))
	if err != nil {
		t.Fatal(err)
	}
	if third.ID == first.ID || len(runs.submitted) != 2 {
		t.Fatalf("next day not new: %+v", third)
	}
}

func TestTriggerRouterFailureAndAdvance(t *testing.T) {
	store, err := NewFileTriggerStore(filepath.Join(t.TempDir(), "triggers"))
	if err != nil {
		t.Fatal(err)
	}
	runs := &triggerRunsStub{submitErr: errors.New("capacity reached")}
	router, _ := NewTriggerRouter(store, runs, ModeDecision, time.Hour)
	rec, err := router.Handle(context.Background(), firedEvent(MonitorAnnouncement, "0123456789abcdef", "20261023"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != ScheduleFailed || rec.FailureNote == "" {
		t.Fatalf("failure not recorded: %+v", rec)
	}
	// Terminal records stay immutable.
	rec2, err := router.Handle(context.Background(), firedEvent(MonitorAnnouncement, "0123456789abcdef", "20261023"))
	if err != nil || rec2.State != ScheduleFailed {
		t.Fatalf("terminal rewritten: %+v %v", rec2, err)
	}
	// Advance: engine completion completes the trigger.
	healthy, _ := NewFileTriggerStore(filepath.Join(t.TempDir(), "triggers2"))
	runsOK := &triggerRunsStub{}
	router2, _ := NewTriggerRouter(healthy, runsOK, ModeDecision, time.Hour)
	running, err := router2.Handle(context.Background(), firedEvent(MonitorAnnouncement, "0123456789abcdef", "20261025"))
	if err != nil {
		t.Fatal(err)
	}
	if err := router2.Advance(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := healthy.Get(running.ID)
	if err != nil || got.State != ScheduleRunning {
		t.Fatalf("non-terminal advanced: %+v %v", got, err)
	}
	completed := runsOK.records[running.RunID]
	completed.State = RunCompleted
	runsOK.records[running.RunID] = completed
	if err := router2.Advance(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = healthy.Get(running.ID)
	if err != nil || got.State != ScheduleCompleted {
		t.Fatalf("completion not advanced: %+v %v", got, err)
	}
}

func TestFileTriggerStoreValidationAndListing(t *testing.T) {
	store, err := NewFileTriggerStore(filepath.Join(t.TempDir(), "triggers"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	good := TriggerRecord{ID: "0123456789abcdef261023", RuleID: "0123456789abcdef", Symbol: "600519.SH", MonitorKind: MonitorPriceBelow, TradeDate: "20261023", Route: RouteContextRefresh, Reason: "close=1499", State: ScheduleCompleted, CreatedAt: now, UpdatedAt: now}
	if err := store.Save(good); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.ID = "0123"
	if err := store.Save(bad); err == nil {
		t.Fatal("bad id accepted")
	}
	bad = good
	bad.Route = "BREW"
	if err := store.Save(bad); err == nil {
		t.Fatal("bad route accepted")
	}
	records, err := store.List(10)
	if err != nil || len(records) != 1 {
		t.Fatalf("list: %+v %v", records, err)
	}
	if _, err := store.List(0); err == nil {
		t.Fatal("zero limit accepted")
	}
	pending, err := store.ListPending()
	if err != nil || len(pending) != 0 {
		t.Fatalf("completed listed as pending: %+v %v", pending, err)
	}
	// Terminal immutability.
	bad = good
	bad.Reason = "rewritten"
	if err := store.Save(bad); err == nil {
		t.Fatal("terminal trigger mutated")
	}
	if _, err := store.Get("0123456789abcdef261099"); err == nil {
		t.Fatal("missing id lookup accepted")
	}
	// Running records may advance but not rewrite history.
	runningRec := good
	runningRec.ID = "0123456789abcdef261024"
	runningRec.TradeDate = "20261024"
	runningRec.State = ScheduleRunning
	if err := store.Save(runningRec); err != nil {
		t.Fatal(err)
	}

}
