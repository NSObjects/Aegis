package manager

import (
 "context"
 "errors"
 "testing"
 "Aegis/internal/researchlang"
 "Aegis/internal/research/debate"
 "Aegis/internal/research/synthesis"
 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/schema"
)

type languagePlanModel struct{
 outputs []string
 calls int
}
func (m *languagePlanModel) Generate(_ context.Context,_ []*schema.AgenticMessage,_ ...model.Option)(*schema.AgenticMessage,error){
 m.calls++
 text:=m.outputs[len(m.outputs)-1]
 if m.calls<=len(m.outputs){text=m.outputs[m.calls-1]}
 return &schema.AgenticMessage{Role:schema.AgenticRoleTypeAssistant,ContentBlocks:[]*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text:text})}},nil
}
func (m *languagePlanModel) Stream(context.Context,[]*schema.AgenticMessage,...model.Option)(*schema.StreamReader[*schema.AgenticMessage],error){panic("stream unexpected")}

func TestManagerCorrectsEnglishResearchSummary(t *testing.T){
 m:=&languagePlanModel{outputs:[]string{
  `{"symbol":"600519.SH","conclusion":"BALANCED","summary":"Market conditions remain uncertain and require more research evidence.","confidence":0.6}`,
  `{"symbol":"600519.SH","conclusion":"BALANCED","summary":"市场证据存在分歧，仍需继续观察。","confidence":0.6}`,
 }}
 plan,err:=(&ResearchManager{Model:m}).BuildPlan(researchlang.RequireChinese(context.Background()),managerArtifact(),managerDebate())
 if err!=nil{t.Fatal(err)}
 if m.calls!=2||plan.Summary!="市场证据存在分歧，仍需继续观察。"{t.Fatalf("calls=%d plan=%+v",m.calls,plan)}
}
func TestManagerFailsWhenBothAttemptsEnglish(t *testing.T){
 m:=&languagePlanModel{outputs:[]string{`{"symbol":"600519.SH","conclusion":"BALANCED","summary":"Market conditions remain uncertain and require more research evidence.","confidence":0.6}`}}
 _,err:=(&ResearchManager{Model:m}).BuildPlan(researchlang.RequireChinese(context.Background()),managerArtifact(),managerDebate())
 if !errors.Is(err,researchlang.ErrLanguage)||m.calls!=2{t.Fatalf("err=%v calls=%d",err,m.calls)}
}
// Compile-time check: our test model exercises the same typed manager interface.
var _ model.BaseModel[*schema.AgenticMessage]=(*languagePlanModel)(nil)
var _ = debate.SideBull
var _ = synthesis.DomainMarket
