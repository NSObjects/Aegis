package application

import (
 "context"
 "errors"
 "strings"
 "testing"
 "Aegis/internal/portfolio"
)

func TestResearchRerunProvenanceSurvivesRestartAndCreatesFreshStudy(t *testing.T) {
 store:=newStore(t)
 firstExecutor:=&controlledRunExecutor{}
 mgr:=newMgr(t,firstExecutor,store,1,3)
 first,err:=mgr.Submit(context.Background(),runFixture("600519.SH"))
 if err!=nil{t.Fatal(err)}
 first=awaitRun(t,mgr,first.ID,RunCompleted)
 request:=runFixture("600519.SH")
 request.IdempotencyKey="fresh-research-again-0001"
 request.RerunOf=first.ID
 second,err:=mgr.Submit(context.Background(),request)
 if err!=nil{t.Fatal(err)}
 if second.ID==first.ID || second.RerunOf!=first.ID {t.Fatalf("lost rerun origin: %+v",second)}
 repeated,err:=mgr.Submit(context.Background(),request)
 if err!=nil|| repeated.ID!=second.ID {t.Fatalf("idempotency did not reuse new run: %+v %v",repeated,err)}
 second=awaitRun(t,mgr,second.ID,RunCompleted)
 if second.RequestedAsOf.IsZero(){t.Fatal("new study missing independent cutoff")}
 old,err:=mgr.Get(context.Background(),first.ID)
 if err!=nil || old.RerunOf!="" || old.State!=RunCompleted {t.Fatalf("original mutated: %+v %v",old,err)}
 mgr.Close()
 restarted:=newMgr(t,&controlledRunExecutor{},store,1,2)
 saved,err:=restarted.Get(context.Background(),second.ID)
 if err!=nil || saved.RerunOf!=first.ID || saved.State!=RunCompleted {t.Fatalf("rerun provenance lost after restart: %+v %v",saved,err)}
}

func TestResearchRerunRejectsUnknownActiveOrMismatchedParentWithoutNewWork(t *testing.T) {
 store:=newStore(t)
 ex:=&controlledRunExecutor{started:make(chan struct{},1),release:make(chan struct{})}
 mgr:=newMgr(t,ex,store,1,3)
 first,err:=mgr.Submit(context.Background(),runFixture("600519.SH"))
 if err!=nil{t.Fatal(err)}
 <-ex.started
 invalid:=runFixture("600519.SH")
 invalid.RerunOf=first.ID
 if _,err:=mgr.Submit(context.Background(),invalid);!errors.Is(err,ErrInvalidRerunSource){t.Fatalf("active parent accepted: %v",err)}
 invalid.RerunOf=strings.Repeat("f",32)
 if _,err:=mgr.Submit(context.Background(),invalid);!errors.Is(err,ErrInvalidRerunSource){t.Fatalf("nonexistent parent accepted: %v",err)}
 invalid.RerunOf="../bad"
 if _,err:=mgr.Submit(context.Background(),invalid);!errors.Is(err,ErrInvalidRerunSource){t.Fatalf("malformed parent accepted: %v",err)}
 close(ex.release)
 _=awaitRun(t,mgr,first.ID,RunCompleted)
 invalid=runFixture("000001.SZ")
 invalid.RerunOf=first.ID
 if _,err:=mgr.Submit(context.Background(),invalid);!errors.Is(err,ErrInvalidRerunSource){t.Fatalf("cross symbol parent accepted: %v",err)}
 invalid=runFixture("600519.SH")
 invalid.Mode=ModePortfolio
 invalid.Portfolio=nil // bad account first
 invalid.RerunOf=first.ID
 if _,err:=mgr.Submit(context.Background(),invalid);err==nil {t.Fatal("missing explicit portfolio accepted")}
 invalid.Portfolio=&portfolio.Context{State:portfolio.StateNotProvided}
 if _,err:=mgr.Submit(context.Background(),invalid);!errors.Is(err,ErrInvalidRerunSource){t.Fatalf("cross mode parent accepted: %v",err)}
 all,err:=mgr.List(context.Background())
 if err!=nil||len(all)!=1||ex.count()!=1{t.Fatalf("invalid reruns triggered side effects: %d %v %d",len(all),err,ex.count())}
}

func TestResearchRerunIdempotencyDoesNotRebindDifferentParent(t *testing.T) {
 store:=newStore(t)
 mgr:=newMgr(t,&controlledRunExecutor{},store,2,4)
 x,err:=mgr.Submit(context.Background(),runFixture("600519.SH"));if err!=nil{t.Fatal(err)}
 y,err:=mgr.Submit(context.Background(),runFixture("600519.SH"));if err!=nil{t.Fatal(err)}
 _=awaitRun(t,mgr,x.ID,RunCompleted)
 _=awaitRun(t,mgr,y.ID,RunCompleted)
 req:=runFixture("600519.SH")
 req.RerunOf=x.ID
 req.IdempotencyKey="rerun-key-conflict-test"
 a,err:=mgr.Submit(context.Background(),req);if err!=nil{t.Fatal(err)}
 req.RerunOf=y.ID
 if _,err:=mgr.Submit(context.Background(),req);!errors.Is(err,ErrRunIdempotencyConflict){t.Fatalf("rerun source rebound for same key: %v",err)}
 _=awaitRun(t,mgr,a.ID,RunCompleted)
}
