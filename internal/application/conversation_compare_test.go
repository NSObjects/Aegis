package application

import (
 "context"
 "encoding/json"
 "testing"
 "time"
)

func TestConversationComparePersistedDecisionRuns(t *testing.T) {
 asOf:=time.Date(2026,10,19,0,0,0,0,time.UTC)
 first:=RunRecord{ID:"left",State:RunCompleted,Symbol:"600519.SH",Mode:ModeDecision,RequestedAsOf:asOf,Artifact:json.RawMessage(`{"symbol":"600519.SH","action":"WATCH","market_view":"偏弱"}`)}
 second:=RunRecord{ID:"right",State:RunCompleted,Symbol:"600519.SH",Mode:ModeDecision,RequestedAsOf:asOf.Add(24*time.Hour),Artifact:json.RawMessage(`{"symbol":"600519.SH","action":"AVOID","market_view":"风险上升"}`)}
 stub:=&conversationRunsStub{records:map[string]RunRecord{"left":first,"right":second}}
 tools:=&ConversationResearchTools{runs:stub}
 got,err:=tools.CompareResearchRuns(context.Background(),"left","right")
 if err!=nil || got.Symbol!="600519.SH" || got.SameResearchDate || len(got.Differences)!=2 || len(got.Caveats)<2 {t.Fatalf("comparison: %+v %v",got,err)}
 for _,tc:=range []struct{name string; change func()}{
  {"same run",func(){}},
  {"pending",func(){v:=stub.records["right"];v.State=RunPending;stub.records["right"]=v}},
  {"mismatched ticker",func(){v:=stub.records["right"];v.Symbol="000001.SZ";stub.records["right"]=v}},
  {"mismatched artifact",func(){v:=stub.records["right"];v.Artifact=json.RawMessage(`{"symbol":"000001.SZ","action":"AVOID"}`);stub.records["right"]=v}},
  {"unsupported review",func(){v:=stub.records["right"];v.Mode=ModePortfolio;stub.records["right"]=v}},
 } {
  stub.records["right"]=second
  tc.change()
  right:="right";if tc.name=="same run"{right="left"}
  if _,err:=tools.CompareResearchRuns(context.Background(),"left",right);err==nil{t.Fatalf("%s must fail closed",tc.name)}
 }
}
