package application

import (
 "context"
 "testing"
 "time"

 "Aegis/internal/decision"
 "Aegis/internal/research"
)

// These tests exercise the real RunManager event sink under cancellation,
// concurrent Runs and explicit reruns; they do not invoke paid providers.
type toolIsolationExecutor struct {
 entered chan string
 release chan struct{}
}
func (e *toolIsolationExecutor) Run(ctx context.Context, req ResearchRequest) (*ResearchResponse,error) {
 name:="snapshot_official"
 research.ReportToolObservation(ctx,research.ToolObservation{Stage:"Event",Tool:name,Status:"START"})
 e.entered<-req.Input.Symbol
 if req.Input.Symbol=="600519.SH" {
  select { case <-ctx.Done(): return nil,ctx.Err(); case <-e.release: }
 } else {
  select { case <-ctx.Done(): return nil,ctx.Err(); default: }
 }
 research.ReportToolObservation(ctx,research.ToolObservation{Stage:"Event",Tool:name,Status:"DONE"})
 return &ResearchResponse{Mode:req.Mode,Decision:&decision.Decision{
  Symbol:req.Input.Symbol,Action:decision.ActionWatch,MarketView:"fixture",Evidence:[]string{"fixture"},
 }},nil
}

func TestToolEventsCancellationIsolationAndRerun(t *testing.T) {
 store:=newStore(t)
 ex:=&toolIsolationExecutor{entered:make(chan string,4),release:make(chan struct{})}
 mgr:=newMgr(t,ex,store,2,4)
 first,err:=mgr.Submit(context.Background(),runFixture("600519.SH"))
 if err!=nil {t.Fatal(err)}
 select {case <-ex.entered: case <-time.After(3*time.Second):t.Fatal("first not started")}
 second,err:=mgr.Submit(context.Background(),runFixture("000001.SZ"))
 if err!=nil {t.Fatal(err)}
 doneSecond:=awaitRun(t,mgr,second.ID,RunCompleted)
 if err:=mgr.Cancel(context.Background(),first.ID);err!=nil {t.Fatal(err)}
 doneFirst:=awaitRun(t,mgr,first.ID,RunCancelled)
 check:=func(rec RunRecord,wantStatus string){
  t.Helper()
  var tools []RunEvent
  for _,ev:=range rec.Events {
   if ev.Kind=="TOOL" { tools=append(tools,ev); if ev.Stage!="Event" || ev.Tool!="snapshot_official" {t.Fatalf("cross-run event: %+v",ev)}}
  }
  if len(tools)==0 || tools[0].Status!="START" {t.Fatalf("missing tool start for %s: %+v",rec.ID,rec.Events)}
  last:=tools[len(tools)-1].Status
  if last!=wantStatus {t.Fatalf("run %s has tool terminal %q, want %q: %+v",rec.ID,last,wantStatus,tools)}
  for i,ev:=range rec.Events {if ev.Seq!=uint64(i+1){t.Fatalf("broken sequence: %+v",rec.Events)}}
 }
 check(doneFirst,"START") // cancelled tool call did not claim success.
 check(doneSecond,"DONE")
 closed,err:=mgr.Get(context.Background(),first.ID)
 if err!=nil || closed.State!=RunCancelled {t.Fatalf("cancelled Run changed: %+v %v",closed,err)}
 // A new study explicitly references the cancelled original but gets a
 // distinct identity and its own TOOL sequence, never a replay of old events.
 rerun:=runFixture("000001.SZ")
 rerun.RerunOf=second.ID
 next,err:=mgr.Submit(context.Background(),rerun)
 if err!=nil {t.Fatal(err)}
 doneNext:=awaitRun(t,mgr,next.ID,RunCompleted)
 if next.ID==second.ID || doneNext.RerunOf!=second.ID {t.Fatalf("rerun identity missing: %+v",doneNext)}
 check(doneNext,"DONE")
 mgr.Close()
 restarted:=newMgr(t,&controlledRunExecutor{},store,1,2)
 for _,rec:=range []RunRecord{doneFirst,doneSecond,doneNext} {
  recovered,err:=restarted.Get(context.Background(),rec.ID)
  if err!=nil || len(recovered.Events)!=len(rec.Events) {t.Fatalf("restart changed events of %s: %+v %v",rec.ID,recovered,err)}
 }
}
