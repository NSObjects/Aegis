package application

import (
 "context"
 "testing"
 "time"
)

func TestRunDeadlinePersistsFailureInsteadOfCancellation(t *testing.T) {
 store:=newStore(t)
 executor:=&controlledRunExecutor{release:make(chan struct{})}
 manager:=newMgr(t,executor,store,1,2)
 if err:=manager.SetRunTimeout(40*time.Millisecond);err!=nil {t.Fatal(err)}
 submitted,err:=manager.Submit(context.Background(),runFixture("600519.SH"))
 if err!=nil {t.Fatal(err)}
 result:=awaitRun(t,manager,submitted.ID,RunFailed)
 if result.FailureCode!=FailureTimeout || result.FailureCategory!="TIMEOUT" || result.FinishedAt==nil {t.Fatalf("missing durable timeout: %+v",result)}
 reopened,err:=NewFileRunStore(store.dir);if err!=nil {t.Fatal(err)}
 saved,err:=reopened.Get(context.Background(),submitted.ID);if err!=nil {t.Fatal(err)}
 if saved.FailureCode!=FailureTimeout || saved.FailureCategory!="TIMEOUT" {t.Fatalf("timeout lost on reopening: %+v",saved)}
}

func TestRunTimeoutConfigurationRejectsInvalidDuration(t *testing.T) {
 manager:=newMgr(t,&controlledRunExecutor{},newStore(t),1,2)
 if manager.SetRunTimeout(0)==nil || manager.SetRunTimeout(-time.Second)==nil {t.Fatal("invalid timeout accepted")}
}
