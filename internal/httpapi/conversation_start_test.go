package httpapi

import (
 "context"
 "net/http"
 "testing"
 "strings"

 "Aegis/internal/application"
)
type authorizedConversationFixture struct { testConversationResponder; starts int; symbol string }
func (r *authorizedConversationFixture) StartConfirmedResearch(_ context.Context,_ string,symbol string)(application.ConversationRunSummary,error){r.starts++;r.symbol=symbol;return application.ConversationRunSummary{RunID:"run-real-123",Symbol:symbol,State:application.RunPending},nil}

func TestConversationStartRequiresExplicitMatchingConfirmation(t *testing.T) {
 h:=mkHandler(t,&fakeRuns{},application.ModeDecision)
 fixture:=&authorizedConversationFixture{}
 h.WithConversationResponder(fixture)
 path:="/api/v1/conversation/query"
 cases:=[]string{
  `{"message":"研究股票","start_symbol":"600519.SH"}`,
  `{"message":"研究股票","confirm_start":true}`,
  `{"message":"研究股票","start_symbol":"000001.SH","confirm_start":true}`,
  `{"message":"研究股票","start_symbol":"600519.SH","confirm_start":false}`,
 }
 for _,body:=range cases {
  res:=apiRequest(t,h,http.MethodPost,path,body,true,nil)
  if res.Code!=http.StatusBadRequest{t.Fatalf("unconfirmed or invalid request accepted: %d %s",res.Code,body)}
 }
 if fixture.starts!=0||fixture.calls!=0{t.Fatal("invalid request reached model or submission")}
 res:=apiRequest(t,h,http.MethodPost,path,`{"message":"研究600519.SH","start_symbol":"600519.SH","confirm_start":true}`,true,nil)
 if res.Code!=http.StatusOK||fixture.starts!=1||fixture.symbol!="600519.SH"||!strings.Contains(res.Body.String(),`"run_id":"run-real-123"`){t.Fatalf("authorized call: %d %s %+v",res.Code,res.Body.String(),fixture)}
 res=apiRequest(t,h,http.MethodPost,path,`{"message":"查看旧报告"}`,true,nil)
 if res.Code!=http.StatusOK||fixture.calls!=1||fixture.starts!=1{t.Fatal("ordinary request did not retain read-only routing")}
}
