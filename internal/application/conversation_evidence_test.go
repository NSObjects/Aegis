package application

import (
 "context"
 "encoding/json"
 "testing"
)

func TestConversationEvidenceIndexFailsClosed(t *testing.T){
 valid:=RunRecord{ID:"indexed",Symbol:"600519.SH",Mode:ModePortfolio,State:RunCompleted,Artifact:json.RawMessage(`{"research_artifact":{"symbol":"600519.SH"}}`)}
 valid.EvidenceIndex=&RunEvidenceIndex{SchemaVersion:1,Status:"NO_EVENT",RequestedAsOf:"2026-10-20T00:00:00Z",Events:[]RunEvidenceEvent{},DocumentIDs:[]string{}}
 // A synthetic stand-alone index must not pass validation against an artifact without EventContext.
 stub:=&conversationRunsStub{records:map[string]RunRecord{"indexed":valid,"missing":{ID:"missing",Mode:ModePortfolio,State:RunCompleted},"pending":{ID:"pending",Mode:ModePortfolio,State:RunPending}}}
 tools:=&ConversationResearchTools{runs:stub}
 for _,id:=range []string{"indexed","missing","pending","unknown"}{
  if _,err:=tools.ReadEvidenceIndex(context.Background(),id);err==nil{t.Fatalf("%s exposed unverified evidence index",id)}
 }
}
