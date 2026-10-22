package application

import (
 "context"
 "strings"
 "testing"
)

func TestConversationRunnerResponderReturnsGroundedFinalAnswer(t *testing.T){
 ctx:=context.Background()
 stub:=&conversationRunsStub{records:map[string]RunRecord{"saved-run":{ID:"saved-run",Symbol:"600519.SH",State:RunCompleted}}}
 model:=&conversationRoundTripModel{}
 agent,err:=NewConversationAgent(ctx,model,&ConversationResearchTools{runs:stub},nil)
 if err!=nil{t.Fatal(err)}
 responder,err:=NewConversationRunnerResponder(ctx,agent)
 if err!=nil{t.Fatal(err)}
 answer,err:=responder.Respond(ctx,"查询 saved-run 的研究标的")
 if err!=nil{t.Fatal(err)}
 if model.turns!=2||!model.sawToolResult||!strings.Contains(answer,"saved-run"){t.Fatalf("unverified response: %q, turns=%d sawTool=%v",answer,model.turns,model.sawToolResult)}
}

func TestConversationRunnerResponderRejectsMissingRuntime(t *testing.T){
 if _,err:=NewConversationRunnerResponder(context.Background(),nil);err==nil{t.Fatal("nil agent accepted")}
 var r *ConversationRunnerResponder
 if _,err:=r.Respond(context.Background(),"hello");err==nil{t.Fatal("nil responder accepted")}
}

func TestConversationRunnerResponderCancelledRequestDoesNotReturnAnswer(t *testing.T) {
 base:=context.Background()
 stub:=&conversationRunsStub{records:map[string]RunRecord{"saved-run":{ID:"saved-run",Symbol:"600519.SH",State:RunCompleted}}}
 model:=&conversationRoundTripModel{}
 agent,err:=NewConversationAgent(base,model,&ConversationResearchTools{runs:stub},nil)
 if err!=nil{t.Fatal(err)}
 responder,err:=NewConversationRunnerResponder(base,agent)
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(base)
 cancel()
 answer,err:=responder.Respond(ctx,"查询 saved-run 的研究标的")
 if err==nil || answer!=""{t.Fatalf("cancelled request leaked answer %q, error %v",answer,err)}
}
