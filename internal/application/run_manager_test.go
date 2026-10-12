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
 eventcap "Aegis/internal/capability/event"
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
	if done.ArtifactVersion != 1 || len(done.ArtifactSHA256) != 64 || done.ArtifactSHA256 != artifactDigest(done.Artifact) {
		t.Fatalf("completed run must persist a versioned, checked artifact: %#v",done)
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

type stageReporter struct{ calls int }

func (x *stageReporter) Run(ctx context.Context, req ResearchRequest) (*ResearchResponse, error) {
	x.calls++
	research.ReportProgress(ctx, research.ProgressEvent{Stage: "Market", Status: "START"})
	research.ReportProgress(ctx, research.ProgressEvent{Stage: "Market", Status: "DONE", DurationMS: 12})
	research.ReportProgress(ctx, research.ProgressEvent{Stage: "Portfolio", Status: "START"})
	research.ReportProgress(ctx, research.ProgressEvent{Stage: "Portfolio", Status: "DONE", DurationMS: 4})
	return &ResearchResponse{Mode: ModeDecision, Decision: &decision.Decision{Symbol: req.Input.Symbol, Action: decision.ActionWatch, MarketView: "fixture", Evidence: []string{"fixture"}}}, nil
}
func TestRunStageEventsPersistAndReplayAfterRestart(t *testing.T) {
	store := newStore(t)
	executor := &stageReporter{}
	manager := newMgr(t, executor, store, 1, 2)
	rec, err := manager.Submit(context.Background(), runFixture("600519.SH"))
	if err != nil {
		t.Fatal(err)
	}
	completed := awaitRun(t, manager, rec.ID, RunCompleted)
	if executor.calls != 1 || len(completed.Events) != 7 {
		t.Fatalf("wrong event count %+v", completed.Events)
	}
	expected := []string{"PENDING", "RUNNING", "START", "DONE", "START", "DONE", "COMPLETED"}
	for i, ev := range completed.Events {
		if ev.Seq != uint64(i+1) || ev.Status != expected[i] || ev.At.IsZero() {
			t.Fatalf("bad event %d: %+v", i, ev)
		}
	}
	manager.Close()
	restarted := newMgr(t, &stageReporter{}, store, 1, 2)
	recovered, err := restarted.Get(context.Background(), rec.ID)
	if err != nil || len(recovered.Events) != 7 {
		t.Fatalf("lost stage events after restart: %+v %v", recovered.Events, err)
	}
}
func TestCorruptStageEventSequenceFailsClosed(t *testing.T) {
	store := newStore(t)
	r := RunRecord{ID: strings.Repeat("c", 32), Mode: ModeDecision, Symbol: "600519.SH", RequestedAsOf: time.Now(), CreatedAt: time.Now(), State: RunPending, Events: []RunEvent{{Seq: 2, Stage: "Run", Status: "PENDING", At: time.Now()}}}
	if err := store.Save(context.Background(), r); err == nil {
		t.Fatal("out of sequence event was accepted")
	}
}

type failingEventStore struct{ RunStore }

func (s failingEventStore) Save(ctx context.Context, rec RunRecord) error {
	if len(rec.Events) > 0 {
		last := rec.Events[len(rec.Events)-1]
		if last.Stage == "Market" && last.Status == "START" {
			return errors.New("simulated disk error")
		}
	}
	return s.RunStore.Save(ctx, rec)
}

type waitingStageExecutor struct{ stopped chan struct{} }

func (x *waitingStageExecutor) Run(ctx context.Context, _ ResearchRequest) (*ResearchResponse, error) {
	research.ReportProgress(ctx, research.ProgressEvent{Stage: "Market", Status: "START"})
	<-ctx.Done()
	close(x.stopped)
	return nil, ctx.Err()
}
func TestRunManagerStopsPaidWorkWhenStagePersistenceFails(t *testing.T) {
	store := failingEventStore{newStore(t)}
	ex := &waitingStageExecutor{stopped: make(chan struct{})}
	m := newMgr(t, ex, store, 1, 2)
	rec, e := m.Submit(context.Background(), runFixture("600519.SH"))
	if e != nil {
		t.Fatal(e)
	}
	done := awaitRun(t, m, rec.ID, RunFailed)
	if done.FailureCode != FailurePersistence || len(done.Artifact) != 0 {
		t.Fatalf("persistence failure not safe: %+v", done)
	}
	select {
	case <-ex.stopped:
	case <-time.After(time.Second):
		t.Fatal("research worker did not receive cancellation after stage persistence failure")
	}
}

func TestRunManagerIdempotencyReusesRunAfterRestart(t *testing.T) {
 store:=newStore(t)
 ex:=&controlledRunExecutor{}
 manager:=newMgr(t,ex,store,1,2)
 req:=runFixture("600519.SH")
 req.IdempotencyKey="browser-retry-key-0001"
 first,err:=manager.Submit(context.Background(),req)
 if err!=nil{t.Fatal(err)}
 done:=awaitRun(t,manager,first.ID,RunCompleted)
 if len(done.IdempotencyDigest)!=64 || len(done.RequestFingerprint)!=64 {
  t.Fatal("idempotency metadata must be hashed and durable")
 }
 // HTTP derives a fresh PIT cutoff on each POST. Retrying the same payload
 // must return the original run rather than paying for new model calls.
 req.Input.RequestedAsOf=req.Input.RequestedAsOf.Add(time.Minute)
 duplicate,err:=manager.Submit(context.Background(),req)
 if err!=nil || duplicate.ID!=first.ID || ex.count()!=1 {
  t.Fatalf("retry created another run: first=%s repeated=%s calls=%d err=%v",first.ID,duplicate.ID,ex.count(),err)
 }
 manager.Close()
 restartedExecutor:=&controlledRunExecutor{}
 restarted:=newMgr(t,restartedExecutor,store,1,2)
 duplicate,err=restarted.Submit(context.Background(),req)
 if err!=nil || duplicate.ID!=first.ID || restartedExecutor.count()!=0 {
  t.Fatalf("restart did not reuse durable run: run=%s calls=%d err=%v",duplicate.ID,restartedExecutor.count(),err)
 }
 req.Input.MarketQuery="different research question"
 if _,err=restarted.Submit(context.Background(),req);!errors.Is(err,ErrRunIdempotencyConflict) {
  t.Fatalf("same key with different request must conflict, got %v",err)
 }
 req.IdempotencyKey="browser-new-run-0002"
 next,err:=restarted.Submit(context.Background(),req)
 if err!=nil || next.ID==first.ID {t.Fatalf("new key must create a new run: %+v %v",next,err)}
 _=awaitRun(t,restarted,next.ID,RunCompleted)
 if restartedExecutor.count()!=1 {t.Fatalf("new research should run exactly once: %d",restartedExecutor.count())}
}

func TestRunManagerConcurrentDuplicateKeyOnlyExecutesOnce(t *testing.T){
 store:=newStore(t)
 ex:=&controlledRunExecutor{started:make(chan struct{},1),release:make(chan struct{})}
 manager:=newMgr(t,ex,store,1,2)
 req:=runFixture("600519.SH")
 req.IdempotencyKey="concurrent-request-123"
 first,err:=manager.Submit(context.Background(),req)
 if err!=nil{t.Fatal(err)}
 <-ex.started
 var wg sync.WaitGroup
 results:=make(chan RunRecord,12)
 errs:=make(chan error,12)
 for i:=0;i<12;i++{
  wg.Add(1)
  go func(){
   defer wg.Done()
   result,e:=manager.Submit(context.Background(),req)
   results<-result
   errs<-e
  }()
 }
 wg.Wait();close(results);close(errs)
 for e:=range errs{if e!=nil{t.Fatalf("duplicate error: %v",e)}}
 for r:=range results{if r.ID!=first.ID{t.Fatalf("duplicate run id %q; expected %q",r.ID,first.ID)}}
 close(ex.release)
 _=awaitRun(t,manager,first.ID,RunCompleted)
 if ex.count()!=1{t.Fatalf("duplicate requests caused %d paid calls",ex.count())}
}

func TestRunManagerIdempotencyRejectsInvalidKeyBeforeExecution(t *testing.T) {
 manager:=newMgr(t,&controlledRunExecutor{},newStore(t),1,2)
 for _,key:=range []string{"short","unexpected whitespace",strings.Repeat("a",129),"ü-invalid-key"}{
  req:=runFixture("600519.SH")
  req.IdempotencyKey=key
  if _,err:=manager.Submit(context.Background(),req);err==nil{t.Fatalf("unsafe key %q accepted",key)}
 }
 records,err:=manager.List(context.Background())
 if err!=nil||len(records)!=0{t.Fatalf("invalid key persisted run: %+v, %v",records,err)}
}

func TestFileStoreRejectsIncompleteIdempotencyMetadata(t *testing.T) {
 store:=newStore(t)
 r:=RunRecord{ID:strings.Repeat("a",32),Mode:ModeDecision,Symbol:"600519.SH",RequestedAsOf:time.Now(),CreatedAt:time.Now(),State:RunPending,IdempotencyDigest:strings.Repeat("0",64)}
 if err:=store.Save(context.Background(),r);err==nil{t.Fatal("incomplete hash fields accepted")}
 r.RequestFingerprint=strings.Repeat("z",64)
 if err:=store.Save(context.Background(),r);err==nil{t.Fatal("malformed fingerprint accepted")}
}

func TestResearchArtifactIntegrityRejectsTamperingAfterRestart(t *testing.T) {
 store:=newStore(t)
 manager:=newMgr(t,&controlledRunExecutor{},store,1,2)
 rec,err:=manager.Submit(context.Background(),runFixture("600519.SH"))
 if err!=nil {t.Fatal(err)}
 completed:=awaitRun(t,manager,rec.ID,RunCompleted)
 manager.Close()
 path:=store.filePath(rec.ID)
 raw,err:=os.ReadFile(path)
 if err!=nil {t.Fatal(err)}
 var document map[string]json.RawMessage
 if err:=json.Unmarshal(raw,&document);err!=nil {t.Fatal(err)}
 document["artifact"]=json.RawMessage(`{"symbol":"000001.SZ","action":"WATCH"}`)
 corrupted,err:=json.Marshal(document)
 if err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(path,corrupted,0600);err!=nil {t.Fatal(err)}
 if _,err:=store.Get(context.Background(),rec.ID);err==nil || !strings.Contains(err.Error(),"checksum mismatch") {
  t.Fatalf("tampering valid JSON must fail closed: %v",err)
 }
 if _,err:=NewRunManager(&controlledRunExecutor{},store,1,2);err==nil || !strings.Contains(err.Error(),"checksum mismatch") {
  t.Fatalf("restart accepted a corrupted completed artifact: %v",err)
 }
 _=completed
}

func TestResearchArtifactLegacyCompatibilityAndVersionRejection(t *testing.T) {
 store:=newStore(t)
 now:=time.Now().UTC()
 base:=RunRecord{
  ID:strings.Repeat("c",32),Mode:ModeDecision,Symbol:"600519.SH",
  RequestedAsOf:now,CreatedAt:now,FinishedAt:&now,State:RunCompleted,
  Artifact:json.RawMessage(`{"symbol":"600519.SH","action":"WATCH"}`),
 }
 // Pre-version data is read without silently inventing a checksum.
 if err:=store.Save(context.Background(),base);err!=nil {t.Fatal(err)}
 legacy,err:=store.Get(context.Background(),base.ID)
 if err!=nil || legacy.ArtifactVersion!=0 || legacy.ArtifactSHA256!="" {
  t.Fatalf("legacy artifact not readable as legacy: %+v, %v",legacy,err)
 }
 base.ArtifactVersion=2
 base.ArtifactSHA256=artifactDigest(base.Artifact)
 if err:=store.Save(context.Background(),base);err==nil || !strings.Contains(err.Error(),"unsupported research artifact version"){
  t.Fatalf("unknown future version must not be accepted: %v",err)
 }
 base.ArtifactVersion=1
 base.ArtifactSHA256=""
 if err:=store.Save(context.Background(),base);err==nil || !strings.Contains(err.Error(),"checksum mismatch"){
  t.Fatalf("new artifact without checksum accepted: %v",err)
 }
}

func TestResearchFailedRunAndEventsPersistAfterRestart(t *testing.T){
 store:=newStore(t)
 ex:=&controlledRunExecutor{err:errors.New("sensitive provider details")}
 manager:=newMgr(t,ex,store,1,2)
 rec,err:=manager.Submit(context.Background(),runFixture("600519.SH"))
 if err!=nil {t.Fatal(err)}
 failed:=awaitRun(t,manager,rec.ID,RunFailed)
 if failed.FailureCode!=FailureResearch || failed.ArtifactVersion!=0 || failed.ArtifactSHA256!="" || len(failed.Events)<3{
  t.Fatalf("invalid failed record: %+v",failed)
 }
 manager.Close()
 restarted:=newMgr(t,&controlledRunExecutor{},store,1,2)
 recovered,err:=restarted.Get(context.Background(),rec.ID)
 if err!=nil || recovered.FailureCode!=FailureResearch || recovered.State!=RunFailed || len(recovered.Events)!=len(failed.Events){
  t.Fatalf("failed run metadata/events lost: %+v %v",recovered,err)
 }
 encoded,_:=json.Marshal(recovered)
 if strings.Contains(string(encoded),"sensitive") || len(recovered.Artifact)!=0 {
  t.Fatalf("failed run exposes secrets or fabricated artifact: %s",encoded)
 }
}

func TestResearchErrorCategoryDoesNotLeakProviderText(t *testing.T) {
	for _, tc := range []struct { err error; want string }{
		{errors.New("generate research synthesis: upstream secret-message"), "SYNTHESIS_MODEL_GENERATE"},
		{errors.New("decode research synthesis JSON: invalid character: secret-message"), "SYNTHESIS_JSON_DECODE"},
		{errors.New("invalid research synthesis artifact: invalid references"), "SYNTHESIS_SCHEMA_VALIDATION"},
		{errors.New("research narrative is not in simplified Chinese: summary"), "LANGUAGE_VALIDATION"},
		{errors.New("provider secret-message"), "UNKNOWN"},
	} {
		if got := researchErrorCategory(tc.err); got != tc.want { t.Errorf("got %q, want %q",got,tc.want) }
	}
}

type observedFixtureRetriever struct { fail bool }
func (r observedFixtureRetriever) ID() string { return "snapshot_official" }
func (r observedFixtureRetriever) Method() eventcap.AcquisitionMethod { return eventcap.AcquisitionAPI }
func (r observedFixtureRetriever) Retrieve(context.Context, eventcap.ResearchTask) ([]eventcap.RawDocument,error) {
 if r.fail { return nil, errors.New("SENSITIVE_PROVIDER_RESPONSE") }
 return nil,nil
}
type observedFixtureExecutor struct { fail bool }
func (e observedFixtureExecutor) Run(ctx context.Context, req ResearchRequest) (*ResearchResponse,error) {
 _,err := (eventcap.AcquisitionPlanRunner{}).Run(ctx,
  eventcap.ResearchTask{ID:"research-test",Symbol:req.Input.Symbol,Objective:"inspect evidence"},
  eventcap.AcquisitionPlan{Steps:[]eventcap.AcquisitionStep{{
   ID:"official",Scope:eventcap.AcquisitionScopeDirect,Retriever:observedFixtureRetriever{fail:e.fail},
  }}},
 )
 if err != nil { return nil,err }
 return &ResearchResponse{Mode:req.Mode,Decision:&decision.Decision{
  Symbol:req.Input.Symbol,Action:decision.ActionWatch,MarketView:"fixture",Evidence:[]string{"fixture"},
 }},nil
}
func TestRunManagerPersistsActualToolEventsAcrossRestart(t *testing.T) {
 for _,tc:=range []struct{name string; fail bool; terminal string}{
  {"success",false,"DONE"},{"retriever_failure",true,"FAIL"},
 } {
  t.Run(tc.name,func(t *testing.T){
   store:=newStore(t)
   mgr:=newMgr(t,observedFixtureExecutor{fail:tc.fail},store,1,2)
   submitted,err:=mgr.Submit(context.Background(),runFixture("600519.SH"))
   if err!=nil {t.Fatal(err)}
   completed:=awaitRun(t,mgr,submitted.ID,RunCompleted)
   var toolEvents []RunEvent
   for i,e:=range completed.Events {
    if e.Seq!=uint64(i+1) { t.Fatalf("noncontiguous sequence: %+v",completed.Events) }
    if e.Kind=="TOOL" {toolEvents=append(toolEvents,e)}
   }
   if len(toolEvents)!=2 || toolEvents[0].Status!="START" || toolEvents[1].Status!=tc.terminal {
    t.Fatalf("unexpected persisted observations: %+v",completed.Events)
   }
   for _,ev:=range toolEvents {
    if ev.Stage!="Event" || ev.Tool!="snapshot_official" {t.Fatalf("wrong tool association: %+v",ev)}
   }
   encoded,_:=json.Marshal(completed.Events)
   if strings.Contains(string(encoded),"SENSITIVE_PROVIDER_RESPONSE") {t.Fatal("secret provider error persisted")}
   mgr.Close()
   restarted:=newMgr(t,&controlledRunExecutor{},store,1,2)
   recovered,err:=restarted.Get(context.Background(),submitted.ID)
   if err!=nil || len(recovered.Events)!=len(completed.Events) {t.Fatalf("restart lost tool events: %v %+v",err,recovered)}
   for i:=range recovered.Events {
    if recovered.Events[i].Seq!=completed.Events[i].Seq ||
      recovered.Events[i].Kind!=completed.Events[i].Kind ||
      recovered.Events[i].Status!=completed.Events[i].Status {t.Fatalf("replay changed events: %+v",recovered.Events)}
   }
  })
 }
}
