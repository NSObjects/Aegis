package httpapi

import (
 "context"
 "encoding/json"
 "net/http"
 "strings"
 "testing"

 "Aegis/internal/application"
)
type testConversationResponder struct { calls int; input string }
func (r *testConversationResponder) Respond(_ context.Context,message string)(string,error){r.calls++;r.input=message;return "已有研究记录可供查询",nil}

func TestConversationHTTPAuthenticationAndFailClosed(t *testing.T){
 h:=mkHandler(t,&fakeRuns{},application.ModeDecision)
 path:="/api/v1/conversation/query"
 request:=`{"message":"查看 600519.SH 研究"}`
 w:=apiRequest(t,h,http.MethodPost,path,request,false,nil)
 if w.Code!=http.StatusUnauthorized{t.Fatalf("unauthenticated: %d",w.Code)}
 w=apiRequest(t,h,http.MethodPost,path,request,true,nil)
 if w.Code!=http.StatusServiceUnavailable{t.Fatalf("missing runtime: %d",w.Code)}
 fake:=&testConversationResponder{}
 h.WithConversationResponder(fake)
 for _,body:=range []string{`{}`,`{"message":""}`,`{"message":"ok","token":"secret"}`,`{"message":"ok"} {}`,`{"message":"`+strings.Repeat("a",2049)+`"}`} {
  w=apiRequest(t,h,http.MethodPost,path,body,true,nil)
  if w.Code!=http.StatusBadRequest{t.Fatalf("invalid body accepted: %d, %.80s",w.Code,body)}
 }
 if fake.calls!=0{t.Fatal("invalid request reached model")}
 w=apiRequest(t,h,http.MethodGet,path,"",true,nil)
 if w.Code!=http.StatusMethodNotAllowed{t.Fatalf("wrong method: %d",w.Code)}
 w=apiRequest(t,h,http.MethodPost,path,request,true,nil)
 if w.Code!=http.StatusOK{t.Fatalf("valid request: %d %s",w.Code,w.Body.String())}
 var result struct{Answer string `json:"answer"`}
 if err:=json.Unmarshal(w.Body.Bytes(),&result);err!=nil||result.Answer!="已有研究记录可供查询"{t.Fatalf("reply: %+v %v",result,err)}
 if fake.calls!=1||fake.input!="查看 600519.SH 研究"{t.Fatalf("unexpected delegation: %+v",fake)}
}
