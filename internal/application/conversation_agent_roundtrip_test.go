package application

import (
 "context"
 "errors"
 "strings"
 "testing"

 "github.com/cloudwego/eino/adk"
 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/schema"
)

type conversationRoundTripModel struct { turns int; sawToolResult bool }
func (m *conversationRoundTripModel) WithTools(_ []*schema.ToolInfo)(model.ToolCallingChatModel,error){return m,nil}
func (m *conversationRoundTripModel) Generate(_ context.Context,messages []*schema.Message,_ ...model.Option)(*schema.Message,error){
 m.turns++
 if m.turns==1{return schema.AssistantMessage("",[]schema.ToolCall{{ID:"read-run-1",Function:schema.FunctionCall{Name:"GetResearchRun",Arguments:`{"run_id":"saved-run"}`}}}),nil}
 for _,message:=range messages {
  if message.Role==schema.Tool && strings.Contains(message.Content,"600519.SH") && strings.Contains(message.Content,"saved-run"){m.sawToolResult=true}
 }
 if !m.sawToolResult{return nil,errors.New("second model turn did not receive the persisted Run tool result")}
 return schema.AssistantMessage("根据已保存的研究 saved-run，标的为 600519.SH。",nil),nil
}
func (m *conversationRoundTripModel) Stream(ctx context.Context,messages []*schema.Message,opts ...model.Option)(*schema.StreamReader[*schema.Message],error){
 response,err:=m.Generate(ctx,messages,opts...)
 if err!=nil{return nil,err}
 return schema.StreamReaderFromArray([]*schema.Message{response}),nil
}

func TestConversationAgentModelToolModelReadsPersistedRun(t *testing.T){
 ctx:=context.Background()
 stub:=&conversationRunsStub{records:map[string]RunRecord{"saved-run":{ID:"saved-run",Symbol:"600519.SH",State:RunCompleted}}}
 fake:=&conversationRoundTripModel{}
 agent,err:=NewConversationAgent(ctx,fake,&ConversationResearchTools{runs:stub},nil)
 if err!=nil{t.Fatal(err)}
 iter:=agent.Run(ctx,&adk.AgentInput{Messages:[]adk.Message{schema.UserMessage("查询 saved-run 的研究标的")}})
 var final string
 for {
  event,ok:=iter.Next();if !ok{break}
  if event.Err!=nil{t.Fatalf("agent loop: %v",event.Err)}
  if event.Output!=nil&&event.Output.MessageOutput!=nil&&event.Output.MessageOutput.Message!=nil{
   msg:=event.Output.MessageOutput.Message
   if msg.Role==schema.Assistant&&len(msg.ToolCalls)==0&&msg.Content!=""{final=msg.Content}
  }
 }
 if fake.turns!=2||!fake.sawToolResult||!strings.Contains(final,"saved-run") {t.Fatalf("roundtrip turns=%d sawSaved=%v final=%q",fake.turns,fake.sawToolResult,final)}
}
