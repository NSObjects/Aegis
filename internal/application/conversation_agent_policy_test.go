package application

import (
 "context"
 "errors"
 "testing"
 "time"

 "github.com/cloudwego/eino/components/model"
 "github.com/cloudwego/eino/schema"
)

type conversationCaptureModel struct { tools []*schema.ToolInfo }
func (m *conversationCaptureModel) Generate(context.Context,[]*schema.Message,...model.Option)(*schema.Message,error){ return nil,errors.New("fixture model must never call a provider") }
func (m *conversationCaptureModel) Stream(context.Context,[]*schema.Message,...model.Option)(*schema.StreamReader[*schema.Message],error){ return nil,errors.New("fixture model must never stream a provider") }
func (m *conversationCaptureModel) WithTools(tools []*schema.ToolInfo)(model.ToolCallingChatModel,error){ m.tools=append([]*schema.ToolInfo(nil),tools...);return m,nil }

func TestConversationAgentToolPolicyExcludesUnapprovedResearchStart(t *testing.T) {
 research:=&ConversationResearchTools{runs:&conversationRunsStub{records:map[string]RunRecord{}}}
 model:=&conversationCaptureModel{}
 a,err:=NewConversationAgent(context.Background(),model,research,nil)
 if err!=nil||a==nil{t.Fatalf("read-only agent: %v",err)}
 if len(model.tools)!=4{t.Fatalf("default agent exposes %d tools, expected only four reads",len(model.tools))}
 for _,info:=range model.tools{
  if info.Name=="StartResearch"{t.Fatal("default read-only agent offered a potentially paid research tool")}
 }
 authorized:=&conversationCaptureModel{}
 a,err=NewConversationAgent(context.Background(),authorized,research,&ConversationStartPolicy{Cutoff:time.Date(2026,10,15,0,0,0,0,time.UTC),Lookback:24*time.Hour})
 if err!=nil||a==nil{t.Fatalf("authorized agent: %v",err)}
 if len(authorized.tools)!=5{t.Fatalf("authorized agent has %d tools, expected five",len(authorized.tools))}
 if authorized.tools[4].Name!="StartResearch"{t.Fatalf("authorized tool mismatch: %s",authorized.tools[4].Name)}
}
