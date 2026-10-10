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
  Instruction:"You are an A-share financial analyst. Analyze only the provided announced income report. Do not create additional accounting numbers, dates or facts. Return a concise interpretation in plain text, including limitations. No JSON required.",
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
 query:=fmt.Sprintf("Research cutoff: %s\nVerified income statement (CNY yuan): %s\nExplain what these numbers imply, and what cannot be inferred.",in.RequestedAsOf,string(facts))
 iter:=a.runner.Query(ctx,query,opts...)
 var interpretation string
 for {
  e,ok:=iter.Next();if !ok{break}
  if e==nil{continue}
  if e.Err!=nil{return nil,fmt.Errorf("fundamental model: %w",e.Err)}
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
 if interpretation==""{return nil,errors.New("fundamental model returned no interpretation")}
 return &Context{Symbol:in.Symbol,RequestedAsOf:in.RequestedAsOf,Income:report,Interpretation:interpretation},nil
}
