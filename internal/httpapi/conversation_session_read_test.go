package httpapi

import (
 "context"
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
