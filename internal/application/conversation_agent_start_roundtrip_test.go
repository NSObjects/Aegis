package application

import (
 "context"
 "errors"
 "strings"
 "testing"
 "time"

 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/schema"
)

type conversationStartLoopModel struct {
 turns int
 calledSymbol string
 sawNewRun bool
}

func (m *conversationStartLoopModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel,error) { return m,nil }
func (m *conversationStartLoopModel) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option)(*schema.Message,error){
 m.turns++
 if m.turns==1 {
  return schema.AssistantMessage("",[]schema.ToolCall{{ID:"start-1",Function:schema.FunctionCall{Name:"StartResearch",Arguments:`{"symbol":"`+m.calledSymbol+`"}`}}}),nil
 }
 for _,msg:=range messages {
  if msg.Role==schema.Tool&&strings.Contains(msg.Content,"new-run"){m.sawNewRun=true}
 }
 if !m.sawNewRun {return nil,errors.New("model never received newly submitted Run ID")}
 return schema.AssistantMessage("研究任务 new-run 已提交，正在等待执行。",nil),nil
}
func (m *conversationStartLoopModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option)(*schema.StreamReader[*schema.Message],error){
 output,err:=m.Generate(ctx,msgs,opts...)
 if err!=nil{return nil,err}
 return schema.StreamReaderFromArray([]*schema.Message{output}),nil
}

func TestConversationAgentAuthorizedStartResearchRoundTrip(t *testing.T){
 ctx:=context.Background()
 stub:=&conversationRunsStub{records:map[string]RunRecord{}}
 fake:=&conversationStartLoopModel{calledSymbol:"600519.SH"}
 cutoff:=time.Date(2026,10,15,9,0,0,0,time.UTC)
 agent,err:=NewConversationAgent(ctx,fake,&ConversationResearchTools{runs:stub},&ConversationStartPolicy{Cutoff:cutoff,Lookback:48*time.Hour,AllowedSymbol:"600519.SH"})
 if err!=nil{t.Fatal(err)}
 reply,err:=NewConversationRunnerResponder(ctx,agent)
 if err!=nil{t.Fatal(err)}
 answer,err:=reply.Respond(ctx,"请开始研究600519.SH")
 if err!=nil{t.Fatal(err)}
 if fake.turns!=2||!fake.sawNewRun||!strings.Contains(answer,"new-run"){t.Fatalf("no grounded new Run reply: turns=%d response=%q",fake.turns,answer)}
 if stub.submitted.Mode!=ModeDecision||stub.submitted.Input.Symbol!="600519.SH"||!stub.submitted.Input.RequestedAsOf.Equal(cutoff)||stub.submitted.Portfolio!=nil{
  t.Fatalf("submitted unsafe research request: %+v",stub.submitted)
 }
}
