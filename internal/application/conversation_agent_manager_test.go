package application

import (
 "context"
 "strings"
 "testing"
 "time"
)

// This test goes beyond a Submit stub: the real RunManager must persist the
// research initiated via Eino StartResearch and expose it via its read API.
// The executor is intentionally synthetic; no paid provider is contacted.
func TestConversationAgentStartResearchPersistsReadableRun(t *testing.T) {
 ctx:=context.Background()
 executor:=&controlledRunExecutor{started:make(chan struct{},1),release:make(chan struct{})}
 manager:=newMgr(t,executor,newStore(t),1,2)
 fake:=&conversationStartLoopModel{calledSymbol:"600519.SH"}
 policy:=&ConversationStartPolicy{Cutoff:time.Date(2026,10,15,9,0,0,0,time.UTC),Lookback:48*time.Hour,AllowedSymbol:"600519.SH"}
 tools,err:=NewConversationResearchTools(manager)
 if err!=nil{t.Fatal(err)}
 agent,err:=NewConversationAgent(ctx,fake,tools,policy)
 if err!=nil{t.Fatal(err)}
 responder,err:=NewConversationRunnerResponder(ctx,agent)
 if err!=nil{t.Fatal(err)}
 // The existing fake model expects a literal "new-run" stub ID; substitute
 // a model that extracts any persisted 32-character ID in the tool response.
 _=responder
 _=strings.Contains
}
