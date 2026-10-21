package application

import (
 "context"
 "encoding/json"
 "errors"
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

// CompareResearchRuns uses original persisted DECISION artifacts only.
// Portfolio review requires its own typed comparison and is rejected rather
// than silently presenting a partial or misleading cross-mode comparison.
func (t *ConversationResearchTools) CompareResearchRuns(ctx context.Context, leftID, rightID string) (ConversationRunComparison,error) {
 if t==nil || t.runs==nil {return ConversationRunComparison{},errors.New("conversation tools unavailable")}
 if leftID=="" || rightID=="" || leftID==rightID {return ConversationRunComparison{},errors.New("two distinct run IDs required")}
 left,err:=t.runs.Get(ctx,leftID);if err!=nil{return ConversationRunComparison{},err}
 right,err:=t.runs.Get(ctx,rightID);if err!=nil{return ConversationRunComparison{},err}
 if left.State!=RunCompleted || right.State!=RunCompleted {return ConversationRunComparison{},errors.New("both runs must be completed")}
 if left.Symbol!=right.Symbol || left.Mode!=right.Mode {return ConversationRunComparison{},errors.New("run symbol or research mode mismatch")}
 if left.Mode!=ModeDecision {return ConversationRunComparison{},errors.New("typed conversation comparison currently supports DECISION runs only")}
 var a,b struct {Symbol string `json:"symbol"`; Action string `json:"action"`; MarketView string `json:"market_view"`}
 if json.Unmarshal(left.Artifact,&a)!=nil || json.Unmarshal(right.Artifact,&b)!=nil || a.Symbol!=left.Symbol || b.Symbol!=right.Symbol || a.Action=="" || b.Action=="" {return ConversationRunComparison{},errors.New("validated decision artifacts unavailable")}
 result:=ConversationRunComparison{LeftRunID:left.ID,RightRunID:right.ID,Symbol:left.Symbol,Mode:left.Mode,LeftAsOf:left.RequestedAsOf.Format("2006-01-02T15:04:05Z07:00"),RightAsOf:right.RequestedAsOf.Format("2006-01-02T15:04:05Z07:00"),Comparable:true, Caveats:[]string{"仅比较已保存研究结论；无法从差异推断变化原因或数据真实性。"}}
 result.SameResearchDate=left.RequestedAsOf.Equal(right.RequestedAsOf)
 if !result.SameResearchDate {result.Caveats=append(result.Caveats,"研究截止时点不同，市场观点不可当作同一时点的严格对照。")}
 if a.Action!=b.Action {result.Differences=append(result.Differences,ConversationComparedField{Field:"action",Left:a.Action,Right:b.Action})}
 if a.MarketView!=b.MarketView {result.Differences=append(result.Differences,ConversationComparedField{Field:"market_view",Left:a.MarketView,Right:b.MarketView})}
 return result,nil
}
