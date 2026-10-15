package application

import (
 "context"
 "encoding/json"
 "errors"
 "testing"
)

type conversationRunsStub struct {
 records map[string]RunRecord
 submitted ResearchRequest
}
func (s *conversationRunsStub) Submit(_ context.Context, req ResearchRequest) (RunRecord,error) {
 s.submitted = req
 return RunRecord{ID:"new-run",Symbol:req.Input.Symbol,State:RunPending},nil
}
func (s *conversationRunsStub) Get(_ context.Context,id string)(RunRecord,error){
 rec,ok:=s.records[id];if !ok{return RunRecord{},ErrRunNotFound};return rec,nil
}
func (s *conversationRunsStub) List(_ context.Context)([]RunRecord,error){
 result:=make([]RunRecord,0,len(s.records));for _,rec:=range s.records {result=append(result,rec)};return result,nil
}

func TestConversationResearchToolsReadOnlyAndFailClosed(t *testing.T) {
 stub:=&conversationRunsStub{records:map[string]RunRecord{
  "done":{ID:"done",Symbol:"600519.SH",State:RunCompleted,Artifact:json.RawMessage(`{"source":"typed-artifact"}`)},
  "failed":{ID:"failed",Symbol:"000001.SZ",State:RunFailed,FailureCode:FailureResearch},
 }}
 tools:=&ConversationResearchTools{runs:stub}
 context:=context.Background()
 artifact,err:=tools.ReadArtifact(context,"done")
 if err!=nil||string(artifact)!=`{"source":"typed-artifact"}` {t.Fatalf("read persisted artifact: %s, %v",artifact,err)}
 artifact[0]='!'
 again,err:=tools.ReadArtifact(context,"done")
 if err!=nil||string(again)!=`{"source":"typed-artifact"}` {t.Fatal("mutating result affected stored research")}
 if _,err:=tools.ReadArtifact(context,"failed");err==nil{t.Fatal("failed Run must not expose an artifact")}
 if _,err:=tools.ReadArtifact(context,"missing");!errors.Is(err,ErrRunNotFound){t.Fatalf("unknown run should fail closed: %v",err)}
 got,err:=tools.GetResearchRun(context,"failed");if err!=nil||got.FailureCode!=FailureResearch{t.Fatalf("run status: %+v %v",got,err)}
 listed,err:=tools.ListResearchRuns(context);if err!=nil||len(listed)!=2{t.Fatalf("list: %+v %v",listed,err)}
 for _,rec:=range listed {if rec.RunID==""||rec.Symbol==""{t.Fatalf("invalid summary: %+v",rec)}}
 req:=runFixture("600519.SH")
 started,err:=tools.StartResearch(context,req)
 if err!=nil||started.RunID!="new-run"||stub.submitted.Input.Symbol!="600519.SH"{t.Fatalf("start must delegate: %+v %v",started,err)}
}

func TestConversationResearchToolsRequiresManager(t *testing.T) {
 if _,err:=NewConversationResearchTools(nil);err==nil{t.Fatal("nil manager accepted")}
 var tools *ConversationResearchTools
 if _,err:=tools.GetResearchRun(context.Background(),"any");err==nil{t.Fatal("nil tools allowed read")}
}
