package fundamental

import (
 "context"
 "errors"
 "strings"
 "testing"

 cap "Aegis/internal/capability/fundamental"
 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/schema"
)

type scriptedIncomeSource struct{
 report *cap.FinancialStatement
 err error
 calls int
}

func (s *scriptedIncomeSource) LatestIncome(_ context.Context,_ string,_ string)(*cap.FinancialStatement,error){
 s.calls++
 return s.report,s.err
}

type scriptedInterpretationModel struct{
 outputs []string
 calls int
 err error
}

func (m *scriptedInterpretationModel) Generate(_ context.Context,_ []*schema.AgenticMessage,_ ...model.Option)(*schema.AgenticMessage,error){
 m.calls++
 if m.err!=nil{return nil,m.err}
 if m.calls>len(m.outputs){return nil,errors.New("unexpected model call")}
 return &schema.AgenticMessage{Role:schema.AgenticRoleTypeAssistant,ContentBlocks:[]*schema.ContentBlock{
  schema.NewContentBlock(&schema.AssistantGenText{Text:m.outputs[m.calls-1]}),
 }},nil
}

func (m *scriptedInterpretationModel) Stream(context.Context,[]*schema.AgenticMessage,...model.Option)(*schema.StreamReader[*schema.AgenticMessage],error){
 return nil,errors.New("unexpected streaming")
}

func newTestFundamentalAgent(t *testing.T,m *scriptedInterpretationModel,s *scriptedIncomeSource)*Agent{
 t.Helper()
 a,err:=New(context.Background(),s,m)
 if err!=nil{t.Fatal(err)}
 return a
}

func testIncomeStatement()*cap.FinancialStatement{
 return &cap.FinancialStatement{Symbol:"600519.SH",AnnouncementDate:"20260830",PeriodEnd:"20260630",Source:"TUSHARE_INCOME"}
}

func TestFundamentalRetryEmptyInterpretationOnce(t *testing.T){
 m:=&scriptedInterpretationModel{outputs:[]string{"  ","The source reports an announced half-year income statement; no comparison with a previous period can be inferred."}}
 s:=&scriptedIncomeSource{report:testIncomeStatement()}
 a:=newTestFundamentalAgent(t,m,s)
 got,err:=a.Analyze(context.Background(),Input{Symbol:"600519.SH",RequestedAsOf:"20261010"})
 if err!=nil{t.Fatal(err)}
 if m.calls!=2 || s.calls!=1{t.Fatalf("want exactly one retry without a second source fetch: model=%d source=%d",m.calls,s.calls)}
 if got.Income!=s.report || !strings.Contains(got.Interpretation,"no comparison"){
  t.Fatalf("must preserve source-owned statement and successful retry: %+v",got)
 }
}

func TestFundamentalRetryStillEmptyFailsClosed(t *testing.T){
 m:=&scriptedInterpretationModel{outputs:[]string{""," "}}
 s:=&scriptedIncomeSource{report:testIncomeStatement()}
 a:=newTestFundamentalAgent(t,m,s)
 got,err:=a.Analyze(context.Background(),Input{Symbol:"600519.SH",RequestedAsOf:"20261010"})
 if got!=nil || err==nil || !strings.Contains(err.Error(),"after 2 attempts") {t.Fatalf("empty twice must fail: result=%+v err=%v",got,err)}
 if m.calls!=2 || s.calls!=1{t.Fatalf("unexpected retry count model=%d source=%d",m.calls,s.calls)}
}

func TestFundamentalNoRetryOnProviderError(t *testing.T){
 m:=&scriptedInterpretationModel{err:errors.New("provider unavailable")}
 s:=&scriptedIncomeSource{report:testIncomeStatement()}
 a:=newTestFundamentalAgent(t,m,s)
 got,err:=a.Analyze(context.Background(),Input{Symbol:"600519.SH",RequestedAsOf:"20261010"})
 if got!=nil || err==nil || !strings.Contains(err.Error(),"provider unavailable"){
  t.Fatalf("provider error must be preserved: result=%+v err=%v",got,err)
 }
 if m.calls!=1{t.Fatalf("provider errors must not retry: calls=%d",m.calls)}
}

func TestFundamentalNonemptyInterpretationNoRetry(t *testing.T){
 m:=&scriptedInterpretationModel{outputs:[]string{"The report is historical, not current."}}
 s:=&scriptedIncomeSource{report:testIncomeStatement()}
 got,err:=newTestFundamentalAgent(t,m,s).Analyze(context.Background(),Input{Symbol:"600519.SH",RequestedAsOf:"20261010"})
 if err!=nil || got==nil || m.calls!=1{t.Fatalf("normal response: got=%+v err=%v model calls=%d",got,err,m.calls)}
}
