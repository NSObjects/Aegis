package httpapi

import (
 "context"
 "encoding/json"
 "net/http"
 "strings"
 "testing"
 "time"

 "Aegis/internal/application"
)

type conversationSessionLookupFixture struct {
 testConversationResponder
 binding application.ConversationSessionBinding
}
func (f *conversationSessionLookupFixture) ResolveConversationSession(_ context.Context,id,owner string)(application.ConversationSessionBinding,error){
 if id!=f.binding.SessionID||owner!=f.binding.OwnerID{return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable}
 return f.binding,nil
}
func TestConversationSessionLookupRequiresAuthenticationAndExistingReference(t *testing.T){
 id:=strings.Repeat("a",32)
 fixture:=&conversationSessionLookupFixture{binding:application.ConversationSessionBinding{
  SessionID:id,OwnerID:"single-operator",RunID:strings.Repeat("b",32),Symbol:"600519.SH",
  AsOf:time.Date(2026,10,19,0,0,0,0,time.UTC),
 }}
 handler:=mkHandler(t,&fakeRuns{},application.ModeDecision)
 handler.WithConversationResponder(fixture)
 path:="/api/v1/conversation/sessions/"+id
 ok:=apiRequest(t,handler,http.MethodGet,path,"",true,nil)
 if ok.Code!=http.StatusOK||!strings.Contains(ok.Body.String(),fixture.binding.RunID)||!strings.Contains(ok.Body.String(),"600519.SH"){t.Fatalf("failed authorized lookup: %d %s",ok.Code,ok.Body.String())}
 unauth:=apiRequest(t,handler,http.MethodGet,path,"",false,nil)
 if unauth.Code!=http.StatusUnauthorized{t.Fatalf("unauthorized read accepted: %d",unauth.Code)}
 missing:=apiRequest(t,handler,http.MethodGet,"/api/v1/conversation/sessions/"+strings.Repeat("c",32),"",true,nil)
 if missing.Code!=http.StatusNotFound{t.Fatalf("unknown session accepted: %d",missing.Code)}
 invalid:=apiRequest(t,handler,http.MethodGet,"/api/v1/conversation/sessions/../bad","",true,nil)
 if invalid.Code!=http.StatusBadRequest{t.Fatalf("invalid session accepted: %d",invalid.Code)}
 method:=apiRequest(t,handler,http.MethodPost,path,"",true,nil)
 if method.Code!=http.StatusMethodNotAllowed{t.Fatalf("invalid method accepted: %d",method.Code)}
}

func TestConversationSessionFollowupResolvesPersistedReference(t *testing.T) {
 id:=strings.Repeat("a",32)
 fixture:=&conversationSessionLookupFixture{binding:application.ConversationSessionBinding{
  SessionID:id,OwnerID:"single-operator",RunID:strings.Repeat("b",32),Symbol:"600519.SH",
  AsOf:time.Date(2026,10,19,0,0,0,0,time.UTC),
 }}
 handler:=mkHandler(t,&fakeRuns{},application.ModeDecision)
 handler.WithConversationResponder(fixture)
 response:=apiRequest(t,handler,http.MethodPost,"/api/v1/conversation/query",
  `{"message":"刚才那次研究有什么空头依据？","session_id":"`+id+`"}`,true,nil)
 if response.Code!=http.StatusOK {t.Fatalf("followup status=%d response=%s",response.Code,response.Body.String())}
 if fixture.calls!=1 {t.Fatalf("read-only responder calls=%d",fixture.calls)}
 if !strings.Contains(fixture.input,"run_id="+fixture.binding.RunID) || !strings.Contains(fixture.input,"symbol=600519.SH") || !strings.Contains(fixture.input,"as_of=2026-10-19T00:00:00Z") {t.Fatalf("missing canonical reference: %s",fixture.input)}
 missing:=apiRequest(t,handler,http.MethodPost,"/api/v1/conversation/query",
  `{"message":"刚才那次研究","session_id":"`+strings.Repeat("c",32)+`"}`,true,nil)
 if missing.Code!=http.StatusNotFound||fixture.calls!=1 {t.Fatalf("unbound session called model: %d",missing.Code)}
}

func TestConversationSessionFollowupCannotAuthorizeNewResearch(t *testing.T) {
 id:=strings.Repeat("a",32)
 fixture:=&conversationSessionLookupFixture{binding:application.ConversationSessionBinding{
  SessionID:id,OwnerID:"single-operator",RunID:strings.Repeat("b",32),Symbol:"600519.SH",
  AsOf:time.Date(2026,10,19,0,0,0,0,time.UTC),
 }}
 handler:=mkHandler(t,&fakeRuns{},application.ModeDecision)
 handler.WithConversationResponder(fixture)
 for _,body:=range []string{
  `{"message":"追问","session_id":"`+id+`","start_symbol":"600519.SH","confirm_start":true}`,
  `{"message":"追问","session_id":"`+id+`","confirm_start":true}`,
 } {
  response:=apiRequest(t,handler,http.MethodPost,"/api/v1/conversation/query",body,true,nil)
  if response.Code!=http.StatusBadRequest {t.Fatalf("mixed session/start accepted: %d %s",response.Code,response.Body.String())}
 }
 if fixture.calls!=0 {t.Fatalf("mixed requests reached model: %d",fixture.calls)}
}

func TestConversationSessionCalendarReferenceRequiresRunDisambiguation(t *testing.T) {
 id:=strings.Repeat("a",32)
 fixture:=&conversationSessionLookupFixture{binding:application.ConversationSessionBinding{
  SessionID:id,OwnerID:"single-operator",RunID:strings.Repeat("b",32),Symbol:"600519.SH",
  AsOf:time.Date(2026,10,19,0,0,0,0,time.UTC),
 }}
 handler:=mkHandler(t,&fakeRuns{},application.ModeDecision)
 handler.WithConversationResponder(fixture)
 for _,question:=range []string{"昨天的空头依据是什么？","前天研究结果","上周的报告呢？"} {
  body,_:=json.Marshal(map[string]string{"message":question,"session_id":id})
  response:=apiRequest(t,handler,http.MethodPost,"/api/v1/conversation/query",string(body),true,nil)
  if response.Code!=http.StatusConflict {t.Fatalf("ambiguous date %q status=%d: %s",question,response.Code,response.Body.String())}
 }
 if fixture.calls!=0 {t.Fatalf("unresolved historical date reached model: %d",fixture.calls)}
}

type conversationHistoryFixture struct {
 conversationSessionLookupFixture
 history []application.ConversationSessionBinding
}
func (f *conversationHistoryFixture) ResolveConversationSessionHistory(_ context.Context,id,owner string)([]application.ConversationSessionBinding,error) {
 if id!=f.binding.SessionID||owner!=f.binding.OwnerID{return nil,application.ErrConversationSessionUnavailable}
 return f.history,nil
}

func TestConversationPreviousRunSelectionIsReadOnlyAndCanonical(t *testing.T) {
 id:=strings.Repeat("a",32)
 current:=application.ConversationSessionBinding{SessionID:id,OwnerID:"single-operator",RunID:strings.Repeat("b",32),Symbol:"600519.SH",AsOf:time.Date(2026,10,19,0,0,0,0,time.UTC)}
 previous:=application.ConversationSessionBinding{SessionID:id,OwnerID:"single-operator",RunID:strings.Repeat("c",32),Symbol:"000001.SZ",AsOf:time.Date(2026,10,17,0,0,0,0,time.UTC)}
 fixture:=&conversationHistoryFixture{conversationSessionLookupFixture:conversationSessionLookupFixture{binding:current},history:[]application.ConversationSessionBinding{current,previous}}
 handler:=mkHandler(t,&fakeRuns{},application.ModeDecision)
 handler.WithConversationResponder(fixture)
 path:="/api/v1/conversation/query"
 body,_:=json.Marshal(map[string]string{"message":"上次研究的空头观点呢？","session_id":id})
 response:=apiRequest(t,handler,http.MethodPost,path,string(body),true,nil)
 if response.Code!=http.StatusOK{t.Fatalf("previous Run lookup: %d %s",response.Code,response.Body.String())}
 if fixture.calls!=1||!strings.Contains(fixture.input,"run_id="+previous.RunID)||strings.Contains(fixture.input,"run_id="+current.RunID){t.Fatalf("incorrect previous Run: %q",fixture.input)}
 fixture.calls=0
 fixture.history=[]application.ConversationSessionBinding{current}
 response=apiRequest(t,handler,http.MethodPost,path,string(body),true,nil)
 if response.Code!=http.StatusConflict||fixture.calls!=0{t.Fatalf("missing previous Run guessed: %d calls=%d",response.Code,fixture.calls)}
}
