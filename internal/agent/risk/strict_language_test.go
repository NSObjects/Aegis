package risk

import (
 "context"
 "errors"
 "testing"
 "Aegis/internal/researchlang"
 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/schema"
)

type riskLanguageModel struct{outputs []string;calls int}
func (m *riskLanguageModel) Generate(_ context.Context,_ []*schema.AgenticMessage,_ ...model.Option)(*schema.AgenticMessage,error){
 m.calls++
 out:=m.outputs[len(m.outputs)-1]
 if m.calls<=len(m.outputs){out=m.outputs[m.calls-1]}
 return &schema.AgenticMessage{Role:schema.AgenticRoleTypeAssistant,ContentBlocks:[]*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text:out})}},nil
}
func (m *riskLanguageModel) Stream(context.Context,[]*schema.AgenticMessage,...model.Option)(*schema.StreamReader[*schema.AgenticMessage],error){panic("unexpected stream")}

func TestRiskCorrectsEnglishNarrativeWithoutChangingAccountState(t *testing.T){
 m:=&riskLanguageModel{outputs:[]string{
  `{"symbol":"600519.SH","assessment":"CAUTION","summary":"The supplied portfolio exposure should be reviewed before making any research conclusion.","confidence":0.6}`,
  `{"symbol":"600519.SH","assessment":"CAUTION","summary":"已有持仓，需要复核已观测的风险敞口。","confidence":0.6}`,
 }}
 res,err:=(&Analyst{Model:m}).Review(researchlang.RequireChinese(context.Background()),testIntent(),testPositionsPortfolio())
 if err!=nil{t.Fatal(err)}
 if m.calls!=2||res.Summary!="已有持仓，需要复核已观测的风险敞口。"{t.Fatalf("calls=%d result=%+v",m.calls,res)}
}
func TestRiskRejectsEnglishAfterTwoAttempts(t *testing.T){
 m:=&riskLanguageModel{outputs:[]string{`{"symbol":"600519.SH","assessment":"CAUTION","summary":"The supplied portfolio exposure should be reviewed before making any research conclusion.","confidence":0.6}`}}
 res,err:=(&Analyst{Model:m}).Review(researchlang.RequireChinese(context.Background()),testIntent(),testPositionsPortfolio())
 if res!=nil||!errors.Is(err,researchlang.ErrLanguage)||m.calls!=2{t.Fatalf("res=%+v err=%v calls=%d",res,err,m.calls)}
}
