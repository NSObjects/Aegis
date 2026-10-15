package application

import (
 "context"
 "strings"
 "testing"
 "time"
)

func TestEinoStartResearchRequiresTrustedPolicyAndValidatedSymbol(t *testing.T) {
 stub:=&conversationRunsStub{records:map[string]RunRecord{}}
 tools:=&ConversationResearchTools{runs:stub}
 if _,err:=tools.EinoStartResearchTool(ConversationStartPolicy{});err==nil{t.Fatal("accepted missing server policy")}
 cutoff:=time.Date(2026,10,15,9,0,0,0,time.UTC)
 invokable,err:=tools.EinoStartResearchTool(ConversationStartPolicy{Cutoff:cutoff,Lookback:48*time.Hour})
 if err!=nil{t.Fatal(err)}
 info,err:=invokable.Info(context.Background())
 if err!=nil||info.Name!="StartResearch"{t.Fatalf("unexpected tool: %+v %v",info,err)}
 for _,symbol:=range []string{"INVALID","600519.SZ","000001.SH","688001.SZ"} {
  if _,err:=invokable.InvokableRun(context.Background(),`{"symbol":"`+symbol+`"}`);err==nil{t.Fatalf("accepted invalid symbol %q",symbol)}
 }
 tooLong:=`{"symbol":"600519.SH","market_query":"`+strings.Repeat("a",1025)+`"}`
 if _,err:=invokable.InvokableRun(context.Background(),tooLong);err==nil{t.Fatal("accepted oversized query")}
 out,err:=invokable.InvokableRun(context.Background(),`{"symbol":"600519.SH","market_query":"earnings"}`)
 if err!=nil||!containsJSONField(out,"run_id","new-run"){t.Fatalf("did not delegate: %q %v",out,err)}
 if stub.submitted.Mode!=ModeDecision||stub.submitted.Portfolio!=nil||!stub.submitted.Input.RequestedAsOf.Equal(cutoff)||stub.submitted.Input.MarketQuery!="earnings" {
  t.Fatalf("unsafe request: %+v",stub.submitted)
 }
}
