package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"Aegis/internal/decision"
	"Aegis/internal/portfolio"
	"Aegis/internal/research"
)

type controlledRunExecutor struct {
	mu           sync.Mutex
	calls        int
	started      chan struct{}
	release      chan struct{}
	err          error
	ignoreCancel bool
	seen         ResearchRequest
}

func (x *controlledRunExecutor) Run(ctx context.Context, req ResearchRequest) (*ResearchResponse, error) {
	x.mu.Lock()
	x.calls++
	x.seen = req
	x.mu.Unlock()
	if x.started != nil {
		select {
		case x.started <- struct{}{}:
		default:
		}
	}
	if x.release != nil {
		if x.ignoreCancel {
			<-x.release
		} else {
			select {
			case <-x.release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	if x.err != nil {
		return nil, x.err
	}
	return &ResearchResponse{Mode: req.Mode, Decision: &decision.Decision{Symbol: req.Input.Symbol, Action: decision.ActionWatch, MarketView: "fixture", Evidence: []string{"fixture"}}}, nil
}
func (x *controlledRunExecutor) count() int { x.mu.Lock(); defer x.mu.Unlock(); return x.calls }
func runFixture(symbol string) ResearchRequest {
	return ResearchRequest{Mode: ModeDecision, Input: research.Input{Symbol: symbol, RequestedAsOf: time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)}}
}
func newStore(t *testing.T) *FileRunStore {
	t.Helper()
	s, e := NewFileRunStore(filepath.Join(t.TempDir(), "runs"))
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func newMgr(t *testing.T, ex ResearchExecutor, store RunStore, parallel, maxActive int) *RunManager {
	t.Helper()
	m, e := NewRunManager(ex, store, parallel, maxActive)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(m.Close)
	return m
}
func awaitRun(t *testing.T, m *RunManager, id string, terminal RunState) RunRecord {
	t.Helper()
	end := time.Now().Add(4 * time.Second)
	for time.Now().Before(end) {
		r, err := m.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if r.State == terminal {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	r, _ := m.Get(context.Background(), id)
	t.Fatalf("run %s stuck in %s, expected %s", id, r.State, terminal)
	return RunRecord{}
}
func TestRunManagerSubmitDurableResultAndRestart(t *testing.T) {
	store := newStore(t)
	ex := &controlledRunExecutor{}
	m := newMgr(t, ex, store, 2, 4)
	r, err := m.Submit(context.Background(), runFixture("600519.SH"))
	if err != nil {
		t.Fatal(err)
	}
	if r.State != RunPending || len(r.ID) != 32 || len(r.Artifact) != 0 {
		t.Fatalf("unsafe pending %#v", r)
	}
	done := awaitRun(t, m, r.ID, RunCompleted)
	if ex.count() != 1 || done.StartedAt == nil || done.FinishedAt == nil || done.FailureCode != "" {
		t.Fatalf("bad completed run %#v", done)
	}
	var output decision.Decision
	if e := json.Unmarshal(done.Artifact, &output); e != nil || output.Symbol != "600519.SH" {
		t.Fatalf("invalid artifact %s: %v", done.Artifact, e)
	}
	m.Close()
	restarted := newMgr(t, &controlledRunExecutor{}, store, 1, 2)
	recovered, err := restarted.Get(context.Background(), r.ID)
	if err != nil || recovered.State != RunCompleted || string(recovered.Artifact) != string(done.Artifact) {
		t.Fatalf("lost completed run: %#v %v", recovered, err)
	}
	list, err := restarted.List(context.Background())
	if err != nil || len(list) != 1 || list[0].ID != r.ID {
		t.Fatalf("bad run list: %#v %v", list, err)
	}
	if _, err := m.Submit(context.Background(), runFixture("600519.SH")); !errors.Is(err, ErrRunManagerClosed) {
		t.Fatalf("closed manager accepted task: %v", err)
	}
}
func TestRunManagerCancelDiscardsLateSuccess(t *testing.T) {
	store := newStore(t)
	ex := &controlledRunExecutor{started: make(chan struct{}, 1), release: make(chan struct{}), ignoreCancel: true}
	m := newMgr(t, ex, store, 1, 2)
	r, e := m.Submit(context.Background(), runFixture("600519.SH"))
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-ex.started:
	case <-time.After(time.Second):
		t.Fatal("never started")
	}
	if e := m.Cancel(context.Background(), r.ID); e != nil {
		t.Fatal(e)
	}
	close(ex.release)
	done := awaitRun(t, m, r.ID, RunCancelled)
	if len(done.Artifact) > 0 || done.FailureCode != "" {
		t.Fatalf("canceled run leaked research: %#v", done)
	}
	if !errors.Is(m.Cancel(context.Background(), "unknown"), ErrRunNotActive) {
		t.Fatal("missing run accepted cancellation")
	}
}
func TestRunManagerFailClosedAndCapacity(t *testing.T) {
	store := newStore(t)
	errSource := errors.New("SECRET_TOKEN provider failed")
	ex := &controlledRunExecutor{err: errSource, started: make(chan struct{}, 1), release: make(chan struct{})}
	m := newMgr(t, ex, store, 1, 1)
	r, e := m.Submit(context.Background(), runFixture("600519.SH"))
	if e != nil {
		t.Fatal(e)
	}
	<-ex.started
	if _, err := m.Submit(context.Background(), runFixture("000001.SZ")); !errors.Is(err, ErrRunCapacity) {
		t.Fatalf("overcapacity: %v", err)
	}
	close(ex.release)
	done := awaitRun(t, m, r.ID, RunFailed)
	raw, _ := json.Marshal(done)
	if done.FailureCode != FailureResearch || len(done.Artifact) != 0 || strings.Contains(string(raw), "SECRET_TOKEN") {
		t.Fatalf("leaked failure details: %s", raw)
	}
}
func TestRunManagerRecoversOnlyInterruptedRuns(t *testing.T) {
	store := newStore(t)
	t0 := time.Now().UTC()
	ids := []string{strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)}
	for i, state := range []RunState{RunPending, RunRunning, RunFailed} {
		r := RunRecord{ID: ids[i], Mode: ModeDecision, Symbol: "600519.SH", RequestedAsOf: t0, CreatedAt: t0, State: state}
		if state == RunRunning {
			r.StartedAt = &t0
		}
		if state == RunFailed {
			r.FinishedAt = &t0
			r.FailureCode = FailureResearch
		}
		if err := store.Save(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	m := newMgr(t, &controlledRunExecutor{}, store, 1, 2)
	for i, id := range ids {
		rec, e := m.Get(context.Background(), id)
		if e != nil {
			t.Fatal(e)
		}
		if i < 2 && (rec.State != RunFailed || rec.FailureCode != FailureInterrupted || rec.FinishedAt == nil) {
			t.Fatalf("did not fail interrupted: %#v", rec)
		}
		if i == 2 && rec.FailureCode != FailureResearch {
			t.Fatalf("overwrote completed error: %#v", rec)
		}
	}
}
func TestRunManagerValidatesBeforeSaveOrProvider(t *testing.T) {
	store := newStore(t)
	ex := &controlledRunExecutor{}
	m := newMgr(t, ex, store, 1, 2)
	tests := []ResearchRequest{{}, {Mode: ModeDecision}, {Mode: ModeDecision, Input: research.Input{Symbol: "600519.SH"}}, {Mode: ModePortfolio, Input: runFixture("600519.SH").Input}, {Mode: ModeDecision, Input: runFixture("600519.SH").Input, Portfolio: &portfolio.Context{State: portfolio.StateEmpty}}}
	for i, req := range tests {
		if _, e := m.Submit(context.Background(), req); e == nil {
			t.Fatalf("invalid request %d accepted", i)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := m.Submit(ctx, runFixture("600519.SH")); !errors.Is(e, context.Canceled) {
		t.Fatalf("canceled submit %v", e)
	}
	list, e := m.List(context.Background())
	if e != nil || len(list) != 0 || ex.count() != 0 {
		t.Fatalf("side effects from invalid task %#v: %v", list, e)
	}
}
func TestRunManagerFreezesCallerPortfolioBeforeAsync(t *testing.T) {
	store := newStore(t)
	ex := &controlledRunExecutor{started: make(chan struct{}, 1), release: make(chan struct{})}
	m := newMgr(t, ex, store, 1, 2)
	input := runFixture("600519.SH").Input
	request := ResearchRequest{Mode: ModePortfolio, Input: input, Portfolio: &portfolio.Context{State: portfolio.StateNotProvided}}
	// Fake provides only decision output, intentionally rejected for portfolio.
	r, e := m.Submit(context.Background(), request)
	if e != nil {
		t.Fatal(e)
	}
	request.Portfolio.State = portfolio.StateEmpty
	<-ex.started
	close(ex.release)
	_ = awaitRun(t, m, r.ID, RunFailed)
	ex.mu.Lock()
	seen := ex.seen.Portfolio.State
	ex.mu.Unlock()
	if seen != portfolio.StateNotProvided {
		t.Fatalf("async runner saw caller-mutated account: %s", seen)
	}
}
func TestFileStoreRejectsTraversalCorruptionAndUnsafeDirectory(t *testing.T) {
	store := newStore(t)
	if _, e := store.Get(context.Background(), "../arbitrary"); e == nil {
		t.Fatal("accepted path traversal")
	}
	if _, e := store.Get(context.Background(), strings.Repeat("a", 32)); !errors.Is(e, ErrRunNotFound) {
		t.Fatal("missing should return ErrRunNotFound")
	}
	data := []byte(`{"run_id":"` + strings.Repeat("f", 32) + `","unexpected":"yes"}`)
	if e := os.WriteFile(filepath.Join(store.dir, strings.Repeat("f", 32)+".json"), data, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := store.List(context.Background()); e == nil {
		t.Fatal("corrupt run file silently skipped")
	}
	if _, e := NewRunManager(&controlledRunExecutor{}, store, 1, 2); e == nil {
		t.Fatal("corrupt store accepted")
	}
	d := filepath.Join(t.TempDir(), "unsafe")
	if e := os.Mkdir(d, 0755); e != nil {
		t.Fatal(e)
	}
	if _, e := NewFileRunStore(d); e == nil {
		t.Fatal("world readable directory accepted")
	}
}
func TestStorePreservesUnknownVersusObservedEmptyAccount(t *testing.T) {
	store := newStore(t)
	if e := store.Save(context.Background(), RunRecord{ID: strings.Repeat("a", 32), Mode: ModePortfolio, Symbol: "600519.SH", RequestedAsOf: time.Now(), CreatedAt: time.Now(), State: RunPending}); e != nil {
		t.Fatal(e)
	}
	got, e := store.Get(context.Background(), strings.Repeat("a", 32))
	if e != nil || got.Mode != ModePortfolio || got.State != RunPending {
		t.Fatalf("bad metadata %#v %v", got, e)
	}
	// Portfolio snapshots are deliberately absent from stored metadata; research
	// results may contain exposures only if the validated run completed.
	content, e := os.ReadFile(store.filePath(got.ID))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(content), "portfolio_snapshot") {
		t.Fatalf("account snapshot leaked: %s", content)
	}
}
func TestRunManagerCloseCancelsInFlight(t *testing.T) {
	store := newStore(t)
	ex := &controlledRunExecutor{started: make(chan struct{}, 1), release: make(chan struct{})}
	m := newMgr(t, ex, store, 1, 2)
	rec, e := m.Submit(context.Background(), runFixture("600519.SH"))
	if e != nil {
		t.Fatal(e)
	}
	<-ex.started
	m.Close()
	canceled := awaitRun(t, m, rec.ID, RunCancelled)
	if canceled.Artifact != nil {
		t.Fatal("cancel leaked result")
	}
	if _, e := m.Submit(context.Background(), runFixture("000001.SZ")); !errors.Is(e, ErrRunManagerClosed) {
		t.Fatalf("close did not reject new work: %v", e)
	}
}
func TestFileStoreRejectsInvalidRecord(t *testing.T) {
	store := newStore(t)
	invalid := RunRecord{ID: strings.Repeat("a", 32), Mode: ModeDecision, Symbol: "600519.SH", RequestedAsOf: time.Now(), CreatedAt: time.Now(), State: RunCompleted}
	if e := store.Save(context.Background(), invalid); e == nil {
		t.Fatal("completed without artifact was accepted")
	}
	invalid.State = RunPending
	invalid.Artifact = json.RawMessage(`{"fact":"invented"}`)
	if e := store.Save(context.Background(), invalid); e == nil {
		t.Fatal("pending leaked artifact")
	}
	if _, e := store.Get(context.Background(), fmt.Sprintf("%032x", 4)); !errors.Is(e, ErrRunNotFound) {
		t.Fatal(e)
	}
}
