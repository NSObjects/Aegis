package application

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
 "time"

 "github.com/cloudwego/eino/adk"
 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/schema"
)

// Deterministic LLM fixture drives the real Eino Agent -> tool -> Agent loop.
// This tests wiring and persisted facts, not a hosted model's tool selection.
type conversationCompareRoundTripModel struct {
 turns int
 sawComparedFacts bool
}
func (m *conversationCompareRoundTripModel) WithTools(_ []*schema.ToolInfo)(model.ToolCallingChatModel,error){return m,nil}
func (m *conversationCompareRoundTripModel) Generate(_ context.Context,messages []*schema.Message,_ ...model.Option)(*schema.Message,error){
 m.turns++
 if m.turns==1 {return schema.AssistantMessage("",[]schema.ToolCall{{ID:"compare-1",Function:schema.FunctionCall{Name:"CompareResearchRuns",Arguments:`{"left_run_id":"left","right_run_id":"right"}`}}}),nil}
 for _,msg:=range messages {
  if msg.Role==schema.Tool && strings.Contains(msg.Content,"left") && strings.Contains(msg.Content,"right") &&
   strings.Contains(msg.Content,"WATCH") && strings.Contains(msg.Content,"AVOID") &&
   strings.Contains(msg.Content,"same_research_date") {
   m.sawComparedFacts=true
  }
 }
 if !m.sawComparedFacts{return nil,errors.New("agent did not receive typed persisted comparison")}
 return schema.AssistantMessage("已保存的 left 与 right 决策分别是 WATCH 和 AVOID；时间不同，不能推断原因。",nil),nil
}
func (m *conversationCompareRoundTripModel) Stream(ctx context.Context,messages []*schema.Message,opts ...model.Option)(*schema.StreamReader[*schema.Message],error){
 out,err:=m.Generate(ctx,messages,opts...);if err!=nil{return nil,err}
 return schema.StreamReaderFromArray([]*schema.Message{out}),nil
}
func TestConversationAgentCompareToolRoundTrip(t *testing.T){
 ctx:=context.Background()
 asOf:=time.Date(2026,10,20,0,0,0,0,time.UTC)
 stub:=&conversationRunsStub{records:map[string]RunRecord{
  "left":{ID:"left",State:RunCompleted,Mode:ModeDecision,Symbol:"600519.SH",RequestedAsOf:asOf,Artifact:json.RawMessage(`{"symbol":"600519.SH","action":"WATCH","market_view":"MIXED"}`)},
  "right":{ID:"right",State:RunCompleted,Mode:ModeDecision,Symbol:"600519.SH",RequestedAsOf:asOf.Add(24*time.Hour),Artifact:json.RawMessage(`{"symbol":"600519.SH","action":"AVOID","market_view":"RISK_OFF"}`)},
 }}
 fake:=&conversationCompareRoundTripModel{}
 agent,err:=NewConversationAgent(ctx,fake,&ConversationResearchTools{runs:stub},nil)
 if err!=nil{t.Fatal(err)}
 iter:=agent.Run(ctx,&adk.AgentInput{Messages:[]adk.Message{schema.UserMessage("比较 left 和 right 的历史研究")}})
 var final string
 for {
  event,ok:=iter.Next();if !ok{break}
  if event.Err!=nil{t.Fatalf("Agent loop failed: %v",event.Err)}
  if event.Output!=nil && event.Output.MessageOutput!=nil && event.Output.MessageOutput.Message!=nil {
   msg:=event.Output.MessageOutput.Message
   if msg.Role==schema.Assistant && len(msg.ToolCalls)==0 && msg.Content!="" {final=msg.Content}
  }
 }
 if fake.turns!=2 || !fake.sawComparedFacts || !strings.Contains(final,"left") || !strings.Contains(final,"right") {
  t.Fatalf("comparison roundtrip turns=%d sawFacts=%v final=%q",fake.turns,fake.sawComparedFacts,final)
 }
}
