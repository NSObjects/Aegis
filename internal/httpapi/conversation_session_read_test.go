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

func TestConversationSessionCannotStartWithoutBinder(t *testing.T) {
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
  if response.Code!=http.StatusServiceUnavailable {t.Fatalf("start without binder not rejected: %d %s",response.Code,response.Body.String())}
 }
 if fixture.calls!=0 {t.Fatalf("unavailable starter reached model: %d",fixture.calls)}
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

func TestSelectConversationHistoricalRunByTradingCalendar(t *testing.T) {
 loc,err:=time.LoadLocation("Asia/Shanghai");if err!=nil{t.Fatal(err)}
 now:=time.Date(2026,10,19,12,0,0,0,loc)
 session:=strings.Repeat("a",32)
 current:=application.ConversationSessionBinding{SessionID:session,OwnerID:"single-operator",RunID:strings.Repeat("b",32),AsOf:now}
 yesterday:=application.ConversationSessionBinding{SessionID:session,OwnerID:"single-operator",RunID:strings.Repeat("c",32),AsOf:now.AddDate(0,0,-1)}
 earlier:=application.ConversationSessionBinding{SessionID:session,OwnerID:"single-operator",RunID:strings.Repeat("d",32),AsOf:now.AddDate(0,0,-2)}
 history:=[]application.ConversationSessionBinding{current,yesterday,earlier}
 for _,tc:=range []struct{message,run string}{
  {"昨天的空头论点",yesterday.RunID},{"昨日的报告",yesterday.RunID},
  {"前天的多头依据",earlier.RunID},{"前日的研究",earlier.RunID},
  {"上次的研究",yesterday.RunID},
 } {
  got,err:=selectConversationHistoricalRun(tc.message,history,now)
  if err!=nil||got.RunID!=tc.run {t.Fatalf("%q: %+v %v",tc.message,got,err)}
 }
 duplicate:=append(append([]application.ConversationSessionBinding{},history...),yesterday)
 for _,tc:=range []struct{message string; history []application.ConversationSessionBinding}{
  {"昨天",duplicate},{"上个月",history},{"昨天",history[:1]},
 } {
  if _,err:=selectConversationHistoricalRun(tc.message,tc.history,now);err==nil{t.Fatalf("ambiguous/missing history resolved: %q",tc.message)}
 }
}

func TestSelectConversationHistoricalRunByDateAndTicker(t *testing.T) {
 loc,err:=time.LoadLocation("Asia/Shanghai");if err!=nil{t.Fatal(err)}
 now:=time.Date(2026,10,19,12,0,0,0,loc)
 session:=strings.Repeat("a",32)
 makeBinding:=func(id,symbol string,days int)application.ConversationSessionBinding {
  return application.ConversationSessionBinding{SessionID:session,OwnerID:"single-operator",RunID:strings.Repeat(id,32),Symbol:symbol,AsOf:now.AddDate(0,0,-days)}
 }
 history:=[]application.ConversationSessionBinding{
  makeBinding("a","000001.SZ",0),
  makeBinding("b","600519.SH",1),
  makeBinding("c","000001.SZ",1),
 }
 got,err:=selectConversationHistoricalRun("昨天600519.SH的研究",history,now)
 if err!=nil||got.RunID!=history[1].RunID {t.Fatalf("ticker and date: %+v %v",got,err)}
 for _,message:=range []string{
  "昨天600519.SH和000001.SZ的研究",
  "昨天601398.SH的研究",
 } {
  if _,err:=selectConversationHistoricalRun(message,history,now);err==nil{t.Fatalf("ambiguous or absent ticker accepted: %s",message)}
 }
 history=append(history,makeBinding("d","600519.SH",1))
 if _,err:=selectConversationHistoricalRun("昨天600519.SH的研究",history,now);err==nil{t.Fatal("duplicate same-day ticker accepted")}
}
