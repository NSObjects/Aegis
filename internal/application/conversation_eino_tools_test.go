package application

import (
 "context"
 "encoding/json"
 "testing"
)

func TestConversationEinoReadToolsExecutePersistedFacts(t *testing.T) {
 stub:=&conversationRunsStub{records:map[string]RunRecord{
  "done":{ID:"done",Symbol:"600519.SH",State:RunCompleted,Artifact:json.RawMessage(`{"portfolio_review":{"debate_state":{"marker":"saved"}}}`)},
  "failed":{ID:"failed",Symbol:"000001.SZ",State:RunFailed},
 }}
 tools,err:=(&ConversationResearchTools{runs:stub}).EinoReadTools()
 if err!=nil||len(tools)!=4{t.Fatalf("Eino tools: %d %v",len(tools),err)}
 ctx:=context.Background()
 names:=[]string{"GetResearchRun","ListResearchRuns","ReadArtifact","ReadDebate"}
 for i,tool:=range tools{
  info,e:=tool.Info(ctx)
  if e!=nil||info.Name!=names[i]{t.Fatalf("unexpected tool %d: %+v %v",i,info,e)}
 }
 args:=`{"run_id":"done"}`
 status,err:=tools[0].InvokableRun(ctx,args)
 if err!=nil||!json.Valid([]byte(status))||!containsJSONField(status,"run_id","done"){t.Fatalf("get run: %q %v",status,err)}
 listed,err:=tools[1].InvokableRun(ctx,`{}`)
 if err!=nil||!json.Valid([]byte(listed)){t.Fatalf("list: %q %v",listed,err)}
 artifact,err:=tools[2].InvokableRun(ctx,args)
 if err!=nil||!json.Valid([]byte(artifact))||!containsJSONField(artifact,"run_id","done"){t.Fatalf("artifact: %q %v",artifact,err)}
 debate,err:=tools[3].InvokableRun(ctx,args)
 if err!=nil||!json.Valid([]byte(debate))||!containsJSONField(debate,"run_id","done"){t.Fatalf("debate: %q %v",debate,err)}
 if _,err:=tools[2].InvokableRun(ctx,`{"run_id":"failed"}`);err==nil{t.Fatal("failed run exposed artifact")}
 if _,err:=tools[3].InvokableRun(ctx,`{"run_id":"missing"}`);err==nil{t.Fatal("missing run exposed debate")}
 if _,err:=tools[0].InvokableRun(ctx,`{"run_id":`);err==nil{t.Fatal("malformed model arguments accepted")}
}
func containsJSONField(data,key,want string) bool {
 var v map[string]json.RawMessage
 if json.Unmarshal([]byte(data),&v)!=nil{return false}
 var actual string
 return json.Unmarshal(v[key],&actual)==nil&&actual==want
}
