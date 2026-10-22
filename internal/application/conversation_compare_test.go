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
  {"mixed mode",func(){v:=stub.records["right"];v.Mode=ModePortfolio;stub.records["right"]=v}},
 } {
  stub.records["right"]=second
  tc.change()
  right:="right";if tc.name=="same run"{right="left"}
  if _,err:=tools.CompareResearchRuns(context.Background(),"left",right);err==nil{t.Fatalf("%s must fail closed",tc.name)}
 }
}

func TestConversationComparePortfolioReview(t *testing.T){
 now:=time.Date(2026,10,20,0,0,0,0,time.UTC)
 mk:=func(id,conclusion string)RunRecord {
  return RunRecord{ID:id,Symbol:"600519.SH",Mode:ModePortfolio,State:RunCompleted,RequestedAsOf:now,Artifact:json.RawMessage(`{"research_artifact":{"symbol":"600519.SH","synthesis":{"summary":"`+conclusion+`"}},"portfolio_review":{"research_plan":{"symbol":"600519.SH","conclusion":"REVIEW_REQUIRED"},"risk_context":{"assessment":"LIMITED"},"portfolio_decision":{"disposition":"REVIEW_REQUIRED"}},"final_risk_gate":{"disposition":"BLOCKED"}}`)}
 }
 left,right:=mk("l","旧研究"),mk("r","新研究")
 stub:=&conversationRunsStub{records:map[string]RunRecord{"l":left,"r":right}}
 tools:=&ConversationResearchTools{runs:stub}
 got,err:=tools.CompareResearchRuns(context.Background(),"l","r")
 if err!=nil || len(got.Differences)!=1 || got.Differences[0].Field!="synthesis.summary" || len(got.Caveats)<2 {t.Fatalf("portfolio comparison: %+v %v",got,err)}
 bad:=right
 bad.Artifact=json.RawMessage(`{"research_artifact":{"symbol":"000001.SZ"},"portfolio_review":{}}`)
 stub.records["r"]=bad
 if _,err:=tools.CompareResearchRuns(context.Background(),"l","r");err==nil{t.Fatal("cross symbol portfolio artifact accepted")}
 bad.Artifact=json.RawMessage(`{"research_artifact":{"symbol":"600519.SH"},"portfolio_review":{"research_plan":{"symbol":"000001.SZ"}}}`)
 stub.records["r"]=bad
 if _,err:=tools.CompareResearchRuns(context.Background(),"l","r");err==nil{t.Fatal("cross symbol plan accepted")}
}

func TestConversationComparePortfolioRejectsNullSections(t *testing.T) {
 for _,raw:=range []string{
  "null",
  `{"research_artifact":null,"portfolio_review":{}}`,
  `{"research_artifact":{"symbol":"600519.SH"},"portfolio_review":null}`,
  `{"research_artifact":{"symbol":"600519.SH"}}`,
 } {
  if _,err:=parsePortfolioCompare(json.RawMessage(raw),"600519.SH");err==nil {
   t.Fatalf("accepted absent or null required portfolio sections: %s",raw)
  }
 }
}
