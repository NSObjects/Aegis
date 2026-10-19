package httpapi

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "strings"
 "testing"

 "Aegis/internal/application"
)

type sessionStartFixture struct {
 authorizedConversationFixture
 binding application.ConversationSessionBinding
 bindErr error
}
func (s *sessionStartFixture) StartConfirmedResearch(_ context.Context,_ string,symbol string)(application.ConversationRunSummary,error){
 s.starts++
 return application.ConversationRunSummary{RunID:strings.Repeat("a",32),Symbol:symbol,State:application.RunPending},nil
}
func (s *sessionStartFixture) BindCompletedConversationRun(_ context.Context,sessionID,ownerID,runID string)(application.ConversationSessionBinding,error){
 if s.bindErr!=nil{return application.ConversationSessionBinding{},s.bindErr}
 s.binding=application.ConversationSessionBinding{SessionID:sessionID,OwnerID:ownerID,RunID:runID}
 return s.binding,nil
}
func TestConfirmedConversationReturnsServerBoundSessionID(t *testing.T){
 h:=mkHandler(t,&fakeRuns{},application.ModeDecision)
 fixture:=&sessionStartFixture{}
 h.WithConversationResponder(fixture)
 response:=apiRequest(t,h,http.MethodPost,"/api/v1/conversation/query",`{"message":"研究股票","start_symbol":"600519.SH","confirm_start":true}`,true,nil)
 if response.Code!=http.StatusOK{t.Fatalf("unexpected status %d: %s",response.Code,response.Body.String())}
 var payload map[string]string
 if err:=json.Unmarshal(response.Body.Bytes(),&payload);err!=nil{t.Fatal(err)}
 if application.ValidateConversationSessionID(payload["session_id"])!=nil{t.Fatalf("missing valid server session ID: %+v",payload)}
 if fixture.binding.RunID!=payload["run_id"]||fixture.binding.SessionID!=payload["session_id"]||fixture.binding.OwnerID!="single-operator"||fixture.starts!=1{t.Fatalf("session not bound to created Run: %+v",fixture.binding)}
}

type reusableSessionStartFixture struct {
 sessionStartFixture
 existing string
}
func (s *reusableSessionStartFixture) ResolveConversationSession(_ context.Context,id,owner string)(application.ConversationSessionBinding,error) {
 if id!=s.existing||owner!="single-operator" {return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable}
 return application.ConversationSessionBinding{SessionID:id,OwnerID:owner,RunID:strings.Repeat("c",32)},nil
}
func TestConfirmedResearchReusesOnlyValidatedSession(t *testing.T) {
 id:=strings.Repeat("b",32)
 h:=mkHandler(t,&fakeRuns{},application.ModeDecision)
 fixture:=&reusableSessionStartFixture{existing:id}
 h.WithConversationResponder(fixture)
 path:="/api/v1/conversation/query"
 invalid:=apiRequest(t,h,http.MethodPost,path,`{"message":"研究","start_symbol":"600519.SH","confirm_start":true,"session_id":"`+strings.Repeat("d",32)+`"}`,true,nil)
 if invalid.Code!=http.StatusNotFound||fixture.starts!=0 {t.Fatalf("unknown session started research: %d starts=%d",invalid.Code,fixture.starts)}
 valid:=apiRequest(t,h,http.MethodPost,path,`{"message":"研究","start_symbol":"600519.SH","confirm_start":true,"session_id":"`+id+`"}`,true,nil)
 if valid.Code!=http.StatusOK||fixture.starts!=1 {t.Fatalf("confirmed session start: %d starts=%d %s",valid.Code,fixture.starts,valid.Body.String())}
 var result map[string]string
 if err:=json.Unmarshal(valid.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if result["session_id"]!=id||fixture.binding.SessionID!=id{t.Fatalf("session not reused: %+v %+v",result,fixture.binding)}
}

func TestCreatedRunRemainsTraceableWhenSessionBindingFails(t *testing.T) {
 h:=mkHandler(t,&fakeRuns{},application.ModeDecision)
 fixture:=&sessionStartFixture{bindErr:errors.New("disk write failed")}
 h.WithConversationResponder(fixture)
 response:=apiRequest(t,h,http.MethodPost,"/api/v1/conversation/query",`{"message":"研究股票","start_symbol":"600519.SH","confirm_start":true}`,true,nil)
 if response.Code!=http.StatusOK {t.Fatalf("created Run hidden: %d %s",response.Code,response.Body.String())}
 var result map[string]string
 if err:=json.Unmarshal(response.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if fixture.starts!=1||result["run_id"]!=strings.Repeat("a",32)||result["session_binding"]!="FAILED"||result["session_id"]!="" {
  t.Fatalf("binding failure contract: %+v starts=%d",result,fixture.starts)
 }
 if !strings.Contains(result["answer"],"不要重复启动"){t.Fatalf("missing duplicate warning: %+v",result)}
}
