package application

import (
 "context"
 "testing"
 "Aegis/internal/decision"
 "Aegis/internal/research"
 "Aegis/internal/researchlang"
)
type reportLanguageRunner struct{strict bool}
func (r *reportLanguageRunner) Run(ctx context.Context,_ research.Input)(*decision.Decision,error){
 r.strict=researchlang.Required(ctx)
 return &decision.Decision{Symbol:"600519.SH",MarketView:"研究数据有效。",Evidence:[]string{"存在市场信息。"},Action:decision.ActionWatch,Confidence:0.6},nil
}
func TestResearchServiceEnablesStrictReportLanguage(t *testing.T){
 runner:=&reportLanguageRunner{}
 service,err:=NewResearchService(runner,nil)
 if err!=nil{t.Fatal(err)}
 _,err=service.Run(context.Background(),ResearchRequest{Mode:ModeDecision})
 if err!=nil{t.Fatal(err)}
 if !runner.strict{t.Fatal("new service runs must enable Chinese report guard")}
}
