package application

import (
 "context"
 "encoding/json"
 "path/filepath"
 "strings"
 "testing"
)

// Reopen from a fresh FileRunStore instance, not the original in-memory object.
// This is a disk-level regression test; it is not a Hosted/Web restart claim.
func TestRunManagerFreshDiskStoreReopenCompletedAndFailed(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"runs")
 store,err:=NewFileRunStore(dir)
 if err!=nil {t.Fatal(err)}
 good:=newMgr(t,&controlledRunExecutor{},store,1,2)
 submitted,err:=good.Submit(context.Background(),runFixture("600519.SH"))
 if err!=nil {t.Fatal(err)}
 completed:=awaitRun(t,good,submitted.ID,RunCompleted)
 if completed.ArtifactVersion!=1 || completed.ArtifactSHA256=="" {t.Fatal("missing durable artifact digest")}
 good.Close()

 reopened,err:=NewFileRunStore(dir)
 if err!=nil {t.Fatal(err)}
 manager:=newMgr(t,&controlledRunExecutor{},reopened,1,2)
 persisted,err:=manager.Get(context.Background(),submitted.ID)
 if err!=nil {t.Fatal(err)}
 if persisted.State!=RunCompleted || persisted.ArtifactSHA256!=completed.ArtifactSHA256 || string(persisted.Artifact)!=string(completed.Artifact) {
  t.Fatalf("completed run not stable across file store reopen: %#v",persisted)
 }
 var typed map[string]any
 if err:=json.Unmarshal(persisted.Artifact,&typed);err!=nil{t.Fatal(err)}
 if typed["symbol"]!="600519.SH"{t.Fatalf("artifact symbol changed: %#v",typed)}
 manager.Close()

 failedStore,err:=NewFileRunStore(dir)
 if err!=nil {t.Fatal(err)}
 failing:=newMgr(t,&controlledRunExecutor{err:context.DeadlineExceeded},failedStore,1,2)
 failedSubmit,err:=failing.Submit(context.Background(),runFixture("000001.SZ"))
 if err!=nil {t.Fatal(err)}
 failed:=awaitRun(t,failing,failedSubmit.ID,RunFailed)
 if failed.FailureCode=="" || len(failed.Artifact)>0 {t.Fatalf("invalid failed terminal run: %#v",failed)}
 failing.Close()

 fresh,err:=NewFileRunStore(dir)
 if err!=nil {t.Fatal(err)}
 reader:=newMgr(t,&controlledRunExecutor{},fresh,1,2)
 got,err:=reader.Get(context.Background(),failedSubmit.ID)
 if err!=nil {t.Fatal(err)}
 if got.State!=RunFailed || got.FailureCode!=failed.FailureCode || len(got.Artifact)!=0 {t.Fatalf("failed run changed after reopen: %#v",got)}
 history,err:=reader.List(context.Background())
 if err!=nil {t.Fatal(err)}
 if len(history)!=2 || history[0].ID!=failedSubmit.ID && history[1].ID!=failedSubmit.ID {t.Fatalf("reopened list lost failed run: %#v",history)}
 encoded,_:=json.Marshal(history)
 if strings.Contains(string(encoded),"deadline exceeded") {t.Fatal("raw provider error leaked into stored run")}
}
