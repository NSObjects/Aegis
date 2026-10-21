package application

import (
 "context"
 "encoding/json"
 "testing"
 "time"
)

func TestConversationEinoReadToolsExecutePersistedFacts(t *testing.T) {
 stub:=&conversationRunsStub{records:map[string]RunRecord{
  "done":{ID:"done",Symbol:"600519.SH",State:RunCompleted,Artifact:json.RawMessage(`{"portfolio_review":{"debate_state":{"marker":"saved"}}}`)},
  "failed":{ID:"failed",Symbol:"000001.SZ",State:RunFailed},
 }}
 tools,err:=(&ConversationResearchTools{runs:stub}).EinoReadTools()
 if err!=nil||len(tools)!=6{t.Fatalf("Eino tools: %d %v",len(tools),err)}
 ctx:=context.Background()
 names:=[]string{"GetResearchRun","ListResearchRuns","ReadArtifact","ReadDebate","CompareResearchRuns","ReadEvidenceIndex"}
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

func TestConversationComparisonToolInvokesPersistedRunGuard(t *testing.T) {
 now:=time.Date(2026,10,20,0,0,0,0,time.UTC)
 stub:=&conversationRunsStub{records:map[string]RunRecord{
  "left":{ID:"left",Symbol:"600519.SH",Mode:ModeDecision,State:RunCompleted,RequestedAsOf:now,Artifact:json.RawMessage(`{"symbol":"600519.SH","action":"WATCH","market_view":"MIXED"}`)},
  "right":{ID:"right",Symbol:"600519.SH",Mode:ModeDecision,State:RunCompleted,RequestedAsOf:now.Add(24*time.Hour),Artifact:json.RawMessage(`{"symbol":"600519.SH","action":"AVOID","market_view":"RISK_OFF"}`)},
  "failed":{ID:"failed",Symbol:"600519.SH",Mode:ModeDecision,State:RunFailed},
 }}
 tools,err:=(&ConversationResearchTools{runs:stub}).EinoReadTools()
 if err!=nil{t.Fatal(err)}
 out,err:=tools[4].InvokableRun(context.Background(),`{"left_run_id":"left","right_run_id":"right"}`)
 if err!=nil{t.Fatal(err)}
 var result ConversationRunComparison
 if json.Unmarshal([]byte(out),&result)!=nil || len(result.Differences)!=2 || result.SameResearchDate || result.LeftRunID!="left" || result.RightRunID!="right" {t.Fatalf("comparison tool omitted persisted differences/PIT: %s",out)}
 for _,args:=range []string{`{"left_run_id":"left","right_run_id":"failed"}`,`{"left_run_id":"left","right_run_id":"missing"}`,`{"left_run_id":"left","right_run_id":"left"}`} {
  if _,err:=tools[4].InvokableRun(context.Background(),args);err==nil{t.Fatalf("comparison accepted invalid args: %s",args)}
 }
 if _,err:=tools[5].InvokableRun(context.Background(),`{"run_id":"left"}`);err==nil{t.Fatal("decision-only Run exposed invented evidence index")}
}
