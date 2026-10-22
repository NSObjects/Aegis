package application

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
)

// ConversationRunComparison reports differences observed in two persisted
// completed runs. Differences are not evidence that one event caused another.
type ConversationRunComparison struct {
 LeftRunID string `json:"left_run_id"`
 RightRunID string `json:"right_run_id"`
 Symbol string `json:"symbol"`
 Mode ResearchMode `json:"mode"`
 LeftAsOf string `json:"left_as_of"`
 RightAsOf string `json:"right_as_of"`
 SameResearchDate bool `json:"same_research_date"`
 Comparable bool `json:"comparable"`
 Caveats []string `json:"caveats"`
 Differences []ConversationComparedField `json:"differences"`
}
type ConversationComparedField struct {
 Field string `json:"field"`
 Left string `json:"left"`
 Right string `json:"right"`
}

// CompareResearchRuns uses original persisted typed artifacts; missing values
// are never presented as verified or inferred from model prose.
func (t *ConversationResearchTools) CompareResearchRuns(ctx context.Context, leftID, rightID string) (ConversationRunComparison,error) {
 if t==nil || t.runs==nil {return ConversationRunComparison{},errors.New("conversation tools unavailable")}
 if leftID=="" || rightID=="" || leftID==rightID {return ConversationRunComparison{},errors.New("two distinct run IDs required")}
 left,err:=t.runs.Get(ctx,leftID);if err!=nil{return ConversationRunComparison{},err}
 right,err:=t.runs.Get(ctx,rightID);if err!=nil{return ConversationRunComparison{},err}
 if left.State!=RunCompleted || right.State!=RunCompleted {return ConversationRunComparison{},errors.New("both runs must be completed")}
 if left.Symbol!=right.Symbol || left.Mode!=right.Mode {return ConversationRunComparison{},errors.New("run symbol or research mode mismatch")}
 if left.Mode!=ModeDecision && left.Mode!=ModePortfolio {return ConversationRunComparison{},errors.New("unsupported comparison mode")}
 var a,b struct {Symbol string `json:"symbol"`; Action string `json:"action"`; MarketView string `json:"market_view"`}
 if left.Mode==ModeDecision && (json.Unmarshal(left.Artifact,&a)!=nil || json.Unmarshal(right.Artifact,&b)!=nil || a.Symbol!=left.Symbol || b.Symbol!=right.Symbol || a.Action=="" || b.Action=="") {return ConversationRunComparison{},errors.New("validated decision artifacts unavailable")}
 result:=ConversationRunComparison{LeftRunID:left.ID,RightRunID:right.ID,Symbol:left.Symbol,Mode:left.Mode,LeftAsOf:left.RequestedAsOf.Format("2006-01-02T15:04:05Z07:00"),RightAsOf:right.RequestedAsOf.Format("2006-01-02T15:04:05Z07:00"),Comparable:true, Caveats:[]string{"仅比较已保存研究结论；无法从差异推断变化原因或数据真实性。"}}
 result.SameResearchDate=left.RequestedAsOf.Equal(right.RequestedAsOf)
 if !result.SameResearchDate {result.Caveats=append(result.Caveats,"研究截止时点不同，市场观点不可当作同一时点的严格对照。")}
 if left.Mode==ModeDecision {
  if a.Action!=b.Action {result.Differences=append(result.Differences,ConversationComparedField{Field:"action",Left:a.Action,Right:b.Action})}
  if a.MarketView!=b.MarketView {result.Differences=append(result.Differences,ConversationComparedField{Field:"market_view",Left:a.MarketView,Right:b.MarketView})}
 } else {
  lv,err:=parsePortfolioCompare(left.Artifact,left.Symbol);if err!=nil{return ConversationRunComparison{},err}
  rv,err:=parsePortfolioCompare(right.Artifact,right.Symbol);if err!=nil{return ConversationRunComparison{},err}
  for _,key:=range []string{"synthesis.summary","research_plan.conclusion","trade_intent.status","risk_context.assessment","portfolio_decision.disposition","final_risk_gate.disposition"} {
   if lv[key]!=rv[key] {result.Differences=append(result.Differences,ConversationComparedField{Field:key,Left:lv[key],Right:rv[key]})}
  }
  result.Caveats=append(result.Caveats,"组合研究仅比较已有字段；缺失字段以空值表示，不代表零或无风险。事件证据和金融事实需单独核验。")
 }
 return result,nil
}

func parsePortfolioCompare(raw json.RawMessage,symbol string)(map[string]string,error){
 var root map[string]json.RawMessage
 if json.Unmarshal(raw,&root)!=nil || root==nil{return nil,errors.New("invalid portfolio artifact")}
 var research map[string]json.RawMessage
 if json.Unmarshal(root["research_artifact"],&research)!=nil || research==nil{return nil,errors.New("missing research artifact")}
 var got string
 if json.Unmarshal(research["symbol"],&got)!=nil || got!=symbol{return nil,errors.New("research symbol mismatch")}
 var review map[string]json.RawMessage
 if json.Unmarshal(root["portfolio_review"],&review)!=nil || review==nil{return nil,errors.New("missing portfolio review")}
 var plan map[string]json.RawMessage
 if json.Unmarshal(review["research_plan"],&plan)==nil {
  var planSymbol string
  if rawSymbol,ok:=plan["symbol"];ok && (json.Unmarshal(rawSymbol,&planSymbol)!=nil || planSymbol!=symbol){return nil,errors.New("research plan symbol mismatch")}
 }
 value:=func(section map[string]json.RawMessage,key string)string{
  var x string
  if section==nil || json.Unmarshal(section[key],&x)!=nil{return ""}
  return strings.TrimSpace(x)
 }
 nested:=func(section map[string]json.RawMessage,key string)map[string]json.RawMessage{
  var next map[string]json.RawMessage
  _=json.Unmarshal(section[key],&next)
  return next
 }
 return map[string]string{
  "synthesis.summary":value(nested(research,"synthesis"),"summary"),
  "research_plan.conclusion":value(plan,"conclusion"),
  "trade_intent.status":value(nested(review,"trade_intent"),"status"),
  "risk_context.assessment":value(nested(review,"risk_context"),"assessment"),
  "portfolio_decision.disposition":value(nested(review,"portfolio_decision"),"disposition"),
  "final_risk_gate.disposition":value(nested(root,"final_risk_gate"),"disposition"),
 },nil
}
