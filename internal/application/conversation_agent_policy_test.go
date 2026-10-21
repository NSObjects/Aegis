package application

import (
 "context"
 "errors"
 "testing"
 "time"

 "github.com/cloudwego/eino/adk"
 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/schema"
)

type conversationCaptureModel struct { tools []*schema.ToolInfo }
func (m *conversationCaptureModel) Generate(_ context.Context,_ []*schema.Message, opts ...model.Option)(*schema.Message,error){ m.tools=append([]*schema.ToolInfo(nil),model.GetCommonOptions(nil,opts...).Tools...); return nil,errors.New("fixture model intentionally stops after observing tool options") }
func (m *conversationCaptureModel) Stream(_ context.Context,_ []*schema.Message, opts ...model.Option)(*schema.StreamReader[*schema.Message],error){ m.tools=append([]*schema.ToolInfo(nil),model.GetCommonOptions(nil,opts...).Tools...); return nil,errors.New("fixture model intentionally stops after observing tool options") }
func (m *conversationCaptureModel) WithTools(tools []*schema.ToolInfo)(model.ToolCallingChatModel,error){ m.tools=append([]*schema.ToolInfo(nil),tools...);return m,nil }

func TestConversationAgentToolPolicyExcludesUnapprovedResearchStart(t *testing.T) {
 research:=&ConversationResearchTools{runs:&conversationRunsStub{records:map[string]RunRecord{}}}
 model:=&conversationCaptureModel{}
 a,err:=NewConversationAgent(context.Background(),model,research,nil)
 if err!=nil||a==nil{t.Fatalf("read-only agent: %v",err)}
 drainConversationTestAgent(a)
 if len(model.tools)!=6{t.Fatalf("default agent exposes %d tools, expected six read-only tools",len(model.tools))}
 for _,info:=range model.tools{
  if info.Name=="StartResearch"{t.Fatal("default read-only agent offered a potentially paid research tool")}
 }
 authorized:=&conversationCaptureModel{}
 a,err=NewConversationAgent(context.Background(),authorized,research,&ConversationStartPolicy{Cutoff:time.Date(2026,10,15,0,0,0,0,time.UTC),Lookback:24*time.Hour})
 if err!=nil||a==nil{t.Fatalf("authorized agent: %v",err)}
 drainConversationTestAgent(a)
 if len(authorized.tools)!=7{t.Fatalf("authorized agent has %d tools, expected seven",len(authorized.tools))}
 if authorized.tools[6].Name!="StartResearch"{t.Fatalf("authorized tool mismatch: %s",authorized.tools[6].Name)}
}

// Eino binds ToolCallingChatModel tools lazily when Run executes, not at construction.
func drainConversationTestAgent(agent adk.Agent) {
 iter:=agent.Run(context.Background(),&adk.AgentInput{Messages:[]adk.Message{schema.UserMessage("请查询已有研究")}})
 for {_,ok:=iter.Next();if !ok {break}}
}
