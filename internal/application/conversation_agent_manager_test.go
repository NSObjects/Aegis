package application

import (
 "context"
 "regexp"
 "testing"
 "time"

 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/schema"
)

type persistedStartModel struct { turns int; runID string }
var newRunIDPattern=regexp.MustCompile(`"run_id":"([a-f0-9]{32})"`)
func (m *persistedStartModel) WithTools(_ []*schema.ToolInfo)(model.ToolCallingChatModel,error){return m,nil}
func (m *persistedStartModel) Generate(_ context.Context,msgs []*schema.Message,_ ...model.Option)(*schema.Message,error){
 m.turns++
 if m.turns==1 {return schema.AssistantMessage("",[]schema.ToolCall{{ID:"start-research",Function:schema.FunctionCall{Name:"StartResearch",Arguments:`{"symbol":"600519.SH"}`}}}),nil}
 for _,msg:=range msgs {if msg.Role==schema.Tool {if match:=newRunIDPattern.FindStringSubmatch(msg.Content);len(match)==2{m.runID=match[1]}}}
 return schema.AssistantMessage("已创建研究任务 "+m.runID,nil),nil
}
func (m *persistedStartModel) Stream(ctx context.Context,msgs []*schema.Message,opts ...model.Option)(*schema.StreamReader[*schema.Message],error){
 result,err:=m.Generate(ctx,msgs,opts...);if err!=nil{return nil,err}
 return schema.StreamReaderFromArray([]*schema.Message{result}),nil
}

// RunManager, durable file store and Eino's actual tool loop participate.
// Only the research executor is mocked to avoid paid Hosted data/models.
func TestConversationAgentStartResearchPersistsReadableRun(t *testing.T) {
 ctx:=context.Background()
 executor:=&controlledRunExecutor{started:make(chan struct{},1),release:make(chan struct{})}
 defer close(executor.release)
 manager:=newMgr(t,executor,newStore(t),1,2)
 fake:=&persistedStartModel{}
 cutoff:=time.Date(2026,10,15,9,0,0,0,time.UTC)
 tools,err:=NewConversationResearchTools(manager);if err!=nil{t.Fatal(err)}
 agent,err:=NewConversationAgent(ctx,fake,tools,&ConversationStartPolicy{Cutoff:cutoff,Lookback:48*time.Hour,AllowedSymbol:"600519.SH"})
 if err!=nil{t.Fatal(err)}
 responder,err:=NewConversationRunnerResponder(ctx,agent);if err!=nil{t.Fatal(err)}
 answer,err:=responder.Respond(ctx,"启动600519.SH研究")
 if err!=nil{t.Fatal(err)}
 if fake.turns!=2||len(fake.runID)!=32||answer!="已创建研究任务 "+fake.runID{t.Fatalf("invalid agent run reply: turns=%d id=%q answer=%q",fake.turns,fake.runID,answer)}
 record,err:=manager.Get(ctx,fake.runID)
 if err!=nil||record.Symbol!="600519.SH"||record.Mode!=ModeDecision||!record.RequestedAsOf.Equal(cutoff){t.Fatalf("new run not queryable: %+v %v",record,err)}
 listed,err:=manager.List(ctx)
 if err!=nil||len(listed)!=1||listed[0].ID!=fake.runID{t.Fatalf("new run not listed: %+v %v",listed,err)}
}
