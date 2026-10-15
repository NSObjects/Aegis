package application

import (
 "context"
 "testing"
 "time"
)

func TestConversationAgentRequiresModel(t *testing.T) {
 stub:=&conversationRunsStub{records:map[string]RunRecord{}}
 researchTools:=&ConversationResearchTools{runs:stub}
 if _,err:=NewConversationAgent(context.Background(),nil,researchTools,nil);err==nil{t.Fatal("nil model accepted")}
 if _,err:=NewConversationAgent(nil,nil,researchTools,nil);err==nil{t.Fatal("nil context accepted")}
 if _,err:=NewConversationAgent(context.Background(),nil,nil,nil);err==nil{t.Fatal("nil dependencies accepted")}
 if _,err:=researchTools.EinoStartResearchTool(ConversationStartPolicy{Cutoff:time.Now()});err==nil{t.Fatal("missing lookback accepted")}
}
