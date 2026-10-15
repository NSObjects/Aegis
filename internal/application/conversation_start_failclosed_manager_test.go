package application

import (
 "context"
 "testing"
 "time"
)

// Reject an LLM's substituted ticker before any durable Run is created.
// Real RunManager and FileRunStore ensure authorization failures are side-effect free.
func TestConversationStartRejectedSymbolDoesNotPersistRun(t *testing.T) {
 ctx:=context.Background()
 executor:=&controlledRunExecutor{}
 manager:=newMgr(t,executor,newStore(t),1,2)
 tools,err:=NewConversationResearchTools(manager)
 if err!=nil{t.Fatal(err)}
 start,err:=tools.EinoStartResearchTool(ConversationStartPolicy{
  Cutoff:time.Date(2026,10,15,9,0,0,0,time.UTC),
  Lookback:48*time.Hour,
  AllowedSymbol:"600519.SH",
 })
 if err!=nil{t.Fatal(err)}
 for _,body:=range []string{
  `{"symbol":"000001.SZ"}`,
  `{"symbol":"600519.SZ"}`,
  `{"symbol":"INVALID"}`,
 } {
  if _,err:=start.InvokableRun(ctx,body);err==nil{t.Fatalf("unapproved request %s accepted",body)}
 }
 runs,err:=manager.List(ctx)
 if err!=nil{t.Fatal(err)}
 if len(runs)!=0||executor.count()!=0{t.Fatalf("denied tool invocation persisted %d runs or executed %d research jobs",len(runs),executor.count())}
}
