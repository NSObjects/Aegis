package fundamental

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "strings"

 cap "Aegis/internal/capability/fundamental"
 "github.com/cloudwego/eino/adk"
 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/schema"
)

type Source interface {
 LatestIncome(context.Context,string,string)(*cap.FinancialStatement,error)
}

// Context contains source-owned financial facts and a separately labeled
// model interpretation. The model never supplies or mutates financial numbers.
type Context struct {
 Symbol string `json:"symbol"`
 RequestedAsOf string `json:"requested_as_of"`
 Income *cap.FinancialStatement `json:"income"`
 Interpretation string `json:"interpretation"`
}

type Input struct {
 Symbol string
 RequestedAsOf string
}

type Agent struct {
 source Source
 runner *adk.TypedRunner[*schema.AgenticMessage]
}

func New(ctx context.Context, source Source, m model.BaseModel[*schema.AgenticMessage])(*Agent,error){
 if source==nil{return nil,errors.New("fundamental source is required")}
 if m==nil{return nil,errors.New("fundamental model is required")}
 a,err:=adk.NewTypedChatModelAgent[*schema.AgenticMessage](ctx,&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
  Name:"fundamental_agent",
  Description:"Interprets verified as-of financial statements without modifying source-owned facts.",
  Instruction:"You are an A-share financial analyst. Analyze only the provided announced income report. Do not create additional accounting numbers, dates or facts. Return a concise interpretation in plain text, including limitations. No JSON required. Write the entire analyst interpretation and limitations in Simplified Chinese, retaining original security codes, units, financial identifiers and exact facts.",
  Model:m,
  MaxIterations:4,
 })
 if err!=nil{return nil,fmt.Errorf("create fundamental agent: %w",err)}
 return &Agent{source:source,runner:adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent:a,EnableStreaming:false})},nil
}

func (a *Agent) Analyze(ctx context.Context,in Input,opts ...adk.AgentRunOption)(*Context,error){
 if a==nil||a.runner==nil||a.source==nil{return nil,errors.New("fundamental agent not initialized")}
 report,err:=a.source.LatestIncome(ctx,in.Symbol,in.RequestedAsOf)
 if err!=nil{return nil,fmt.Errorf("fundamental income source: %w",err)}
 if report==nil||report.Symbol!=in.Symbol{return nil,errors.New("fundamental source returned mismatched report")}
 facts,err:=json.Marshal(report);if err!=nil{return nil,err}
 query:=fmt.Sprintf("研究截止：%s\n经核实的利润表（人民币元）：%s\n请用简体中文解释这些数字所能支持的结论和不能推断的事项；保留原始数值与字段，不要补造事实。",in.RequestedAsOf,string(facts))
 interpretation,err:=a.runInterpretation(ctx,query,opts...)
 if err!=nil{return nil,err}
 if interpretation==""{
  // Retry only an empty, otherwise successful model result. The verified
  // financial statement and research cutoff are supplied again unchanged.
  // Provider errors and cancelled contexts never trigger another request.
  if err:=ctx.Err();err!=nil{return nil,err}
  repairQuery:=query+"\n上一次输出为空。请严格依据上述财务报表，使用简体中文给出非空的解释，并明确局限。不要编造金额、比较、趋势或缺失的事实。"
  interpretation,err=a.runInterpretation(ctx,repairQuery,opts...)
  if err!=nil{return nil,fmt.Errorf("fundamental interpretation retry: %w",err)}
 }
 if interpretation==""{return nil,errors.New("fundamental model returned no interpretation after 2 attempts")}
 return &Context{Symbol:in.Symbol,RequestedAsOf:in.RequestedAsOf,Income:report,Interpretation:interpretation},nil
}

func (a *Agent) runInterpretation(ctx context.Context,query string,opts ...adk.AgentRunOption)(string,error){
 iter:=a.runner.Query(ctx,query,opts...)
 var interpretation string
 for {
  e,ok:=iter.Next();if !ok{break}
  if e==nil{continue}
  if e.Err!=nil{return "",fmt.Errorf("fundamental model: %w",e.Err)}
  if e.Output==nil||e.Output.MessageOutput==nil||e.Output.MessageOutput.Message==nil{continue}
  msg:=e.Output.MessageOutput.Message
  if msg.Role!=schema.AgenticRoleTypeAssistant{continue}
  for _,b:=range msg.ContentBlocks{
   if b!=nil&&b.Type==schema.ContentBlockTypeAssistantGenText&&b.AssistantGenText!=nil{
    s:=strings.TrimSpace(b.AssistantGenText.Text)
    if s!=""{interpretation=s}
   }
  }
 }
 return interpretation,nil
}
