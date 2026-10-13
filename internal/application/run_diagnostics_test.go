package application

import (
    "context"
    "errors"
    "strings"
    "testing"
    "time"
)

func TestExplicitFailedStageUsesOnlyStageFAIL(t *testing.T) {
    events := []RunEvent{
        {Seq: 1, Stage: "Event", Status: "START"},
        {Seq: 2, Kind: "TOOL", Stage: "Event", Tool: "tavily_web_search", Status: "DONE"},
    }
    if got := explicitFailedStage(events); got != "" { t.Fatalf("tool DONE is not stage failure: %q", got) }
    events = append(events, RunEvent{Seq: 3, Stage: "Event", Status: "FAIL"})
    if got := explicitFailedStage(events); got != "Event" { t.Fatalf("expected observed Event FAIL, got %q", got) }
    events = append(events, RunEvent{Seq: 4, Stage: "Market", Status: "FAIL"})
    if got := explicitFailedStage(events); got != "Event" { t.Fatalf("expected first stage failure Event, got %q", got) }
}

func TestFailureDiagnosticsStoredAndReopened(t *testing.T) {
    store := newStore(t)
    now := time.Now().UTC()
    r := RunRecord{
        ID: strings.Repeat("a", 32), Mode: ModeDecision, Symbol: "600519.SH",
        RequestedAsOf: now, CreatedAt: now, StartedAt: &now, FinishedAt: &now,
        State: RunFailed, FailureCode: FailureResearch,
        FailureStage: "Event", FailureCategory: "UNKNOWN",
        Events: []RunEvent{
            {Seq: 1, Stage: "Event", Status: "START", At: now},
            {Seq: 2, Stage: "Event", Status: "FAIL", At: now},
        },
    }
    if err := store.Save(context.Background(), r); err != nil { t.Fatal(err) }
    reopened, err := NewFileRunStore(store.dir)
    if err != nil { t.Fatal(err) }
    got, err := reopened.Get(context.Background(), r.ID)
    if err != nil { t.Fatal(err) }
    if got.FailureStage != "Event" || got.FailureCategory != "UNKNOWN" {
        t.Fatalf("diagnostics not persisted: %+v", got)
    }
    r.FailureCategory = "SECRET_PROVIDER_RESPONSE"
    if err := store.Save(context.Background(), r); err == nil {
        t.Fatal("raw diagnostic must never be persisted")
    }
    r.FailureCategory = "UNKNOWN"
    r.FailureStage = "Secret prompt and provider response"
    if err := store.Save(context.Background(), r); err == nil {
        t.Fatal("untrusted stage must never be persisted")
    }
}

func TestResearchFailureDoesNotPersistSensitiveError(t *testing.T) {
    store := newStore(t)
    executor := &controlledRunExecutor{err: &sensitiveTestError{}}
    manager := newMgr(t, executor, store, 1, 1)
    submitted, err := manager.Submit(context.Background(), runFixture("600519.SH"))
    if err != nil { t.Fatal(err) }
    result := awaitRun(t, manager, submitted.ID, RunFailed)
    if result.FailureCategory != "UNKNOWN" || result.FailureStage != "" {
        t.Fatalf("invented failure details: %+v", result)
    }
}

func TestEventFailureCategoryIsPersistable(t *testing.T) {
    store := newStore(t)
    executor := &controlledRunExecutor{err: errors.New("event research: run event evidence research loop: acquisition step failed")}
    manager := newMgr(t, executor, store, 1, 1)
    submitted, err := manager.Submit(context.Background(), runFixture("600519.SH"))
    if err != nil { t.Fatal(err) }
    result := awaitRun(t, manager, submitted.ID, RunFailed)
    if result.FailureCategory != "EVENT_EVIDENCE_LOOP" || result.FailureStage != "" {
        t.Fatalf("event failure diagnostics not persisted: %+v", result)
    }
}

type sensitiveTestError struct{}
func (*sensitiveTestError) Error() string { return "PRIVATE_API_KEY external model failed" }
