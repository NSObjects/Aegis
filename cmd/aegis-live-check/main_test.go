package main

import (
 "bytes"
 "encoding/json"
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

const fakeID="11223344556677889900aabbccddeeff"
func fakeServer(t *testing.T,mode,state string,wrongAccount,brokenSequence bool)(*httptest.Server,*int){
 return fakeServerWithToolEvents(t,mode,state,wrongAccount,brokenSequence,false)
}
func fakeServerWithToolEvents(t *testing.T,mode,state string,wrongAccount,brokenSequence,withTools bool)(*httptest.Server,*int){
 t.Helper()
 downloads:=new(int)
 h:=http.NewServeMux()
 root:="/api/v1/research/runs"
 h.HandleFunc(root,func(w http.ResponseWriter,r *http.Request){
  if r.Method!="POST"{t.Error("expected POST");return}
  var data map[string]any
  if err:=jsonDecode(r,&data);err!=nil{t.Error(err);return}
  if getText(data,"mode")!=mode||getText(data,"symbol")!="600519.SH"{t.Error("wrong research request")}
  if mode=="PORTFOLIO_REVIEW"&&getText(getMap(data,"portfolio"),"state")!="NOT_PROVIDED"{t.Error("portfolio state invented")}
  w.WriteHeader(202)
  fmt.Fprintf(w,"{\"run_id\":%q,\"mode\":%q,\"symbol\":\"600519.SH\",\"state\":\"PENDING\"}",fakeID,mode)
 })
 h.HandleFunc(root+"/"+fakeID+"/stream",func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Content-Type","text/event-stream")
  names:=[]string{"Run","Market","Technical","Event","Synthesis"}
  if mode=="PORTFOLIO_REVIEW"{names=append(names,"Debate","ResearchManager","Trading","Risk","Portfolio","FinalRiskGate")}
  for i,name:=range names{
   seq:=i+1
   if brokenSequence&&i==1{seq=3}
   status:="DONE"
   if i==0{status="PENDING"}
   fmt.Fprintf(w,"id: %d\nevent: research_stage\ndata: {\"seq\":%d,\"stage\":%q,\"status\":%q}\n\n",seq,seq,name,status)
  }
  seq:=len(names)+1
  if withTools {
   for _,tool:=range []struct{stage,name string}{{"Technical","stock_technical_research"},{"Event","snapshot_official"}} {
    for _,status:=range []string{"START","DONE"} {
     fmt.Fprintf(w,"id: %d\nevent: research_stage\ndata: {\"seq\":%d,\"kind\":\"TOOL\",\"stage\":%q,\"tool\":%q,\"status\":%q}\n\n",seq,seq,tool.stage,tool.name,status)
     seq++
    }
   }
  }
  fmt.Fprintf(w,"id: %d\nevent: research_stage\ndata: {\"seq\":%d,\"stage\":\"Run\",\"status\":%q}\n\n",seq,seq,state)
 })
 h.HandleFunc(root+"/"+fakeID,func(w http.ResponseWriter,r *http.Request){
  fmt.Fprintf(w,"{\"run_id\":%q,\"state\":%q,\"failure_code\":\"RESEARCH_FAILED\"}",fakeID,state)
 })
 h.HandleFunc(root+"/"+fakeID+"/artifact",func(w http.ResponseWriter,r *http.Request){
  *downloads = *downloads+1
  if mode=="DECISION"{fmt.Fprint(w,"{\"symbol\":\"600519.SH\",\"market_view\":\"Risk off\",\"action\":\"AVOID\",\"evidence\":[\"source\"]}");return}
  account:="NOT_PROVIDED";if wrongAccount{account="EMPTY"}
  fmt.Fprintf(w,"{\"research_artifact\":{\"symbol\":\"600519.SH\",\"market\":{},\"technical\":{},\"event\":{},\"synthesis\":{}},\"portfolio_review\":{\"portfolio_decision\":{\"portfolio_state\":%q,\"disposition\":\"REVIEW_REQUIRED\"}},\"final_risk_gate\":{\"disposition\":\"REVIEW_REQUIRED\"}}",account)
 })
 return httptest.NewServer(h),downloads
}
func jsonDecode(r *http.Request,v any)error{return json.NewDecoder(r.Body).Decode(v)}
func TestLiveCheckPortfolioContract(t *testing.T){
 server,downloads:=fakeServer(t,"PORTFOLIO_REVIEW","COMPLETED",false,false);defer server.Close()
 var out bytes.Buffer
 err:=verify(context.Background(),server.Client(),options{url:server.URL,symbol:"600519.SH",mode:"PORTFOLIO_REVIEW"},&out)
 if err!=nil||*downloads!=1||!strings.Contains(out.String(),"PASS:"){t.Fatalf("unexpected result %v, %s",err,out.String())}
}
func TestLiveCheckFailureNeverReadsArtifact(t *testing.T){
 server,downloads:=fakeServer(t,"PORTFOLIO_REVIEW","FAILED",false,false);defer server.Close()
 err:=verify(context.Background(),server.Client(),options{url:server.URL,symbol:"600519.SH",mode:"PORTFOLIO_REVIEW"},&bytes.Buffer{})
 if err==nil||*downloads!=0{t.Fatalf("accepted failed research %v, artifacts %d",err,*downloads)}
}
func TestLiveCheckRejectsFabricatedAccount(t *testing.T){
 server,_:=fakeServer(t,"PORTFOLIO_REVIEW","COMPLETED",true,false);defer server.Close()
 err:=verify(context.Background(),server.Client(),options{url:server.URL,symbol:"600519.SH",mode:"PORTFOLIO_REVIEW"},&bytes.Buffer{})
 if err==nil||!strings.Contains(err.Error(),"unknown portfolio"){t.Fatalf("accepted fabricated account: %v",err)}
}
func TestLiveCheckRejectsMissingSSEEvent(t *testing.T){
 server,_:=fakeServer(t,"DECISION","COMPLETED",false,true);defer server.Close()
 err:=verify(context.Background(),server.Client(),options{url:server.URL,symbol:"600519.SH",mode:"DECISION"},&bytes.Buffer{})
 if err==nil||!strings.Contains(err.Error(),"sequence"){t.Fatalf("accepted invalid SSE sequence: %v",err)}
}
func TestLiveCheckRequiresLoopback(t *testing.T){
 err:=verify(context.Background(),&http.Client{},options{url:"http://public.example:8686",symbol:"600519.SH",mode:"DECISION"},&bytes.Buffer{})
 if err==nil{t.Fatal("accepted public endpoint")}
}

func TestLiveCheckStrictObservedToolsRequired(t *testing.T) {
 without,_:=fakeServer(t,"PORTFOLIO_REVIEW","COMPLETED",false,false)
 defer without.Close()
 opts:=options{url:without.URL,symbol:"600519.SH",mode:"PORTFOLIO_REVIEW",requireToolEvents:true}
 if err:=verify(context.Background(),without.Client(),opts,&bytes.Buffer{});err==nil || !strings.Contains(err.Error(),"missing observed tool lifecycle") {
  t.Fatalf("stage-only hosted run must not pass tool acceptance: %v",err)
 }
 with,_:=fakeServerWithToolEvents(t,"PORTFOLIO_REVIEW","COMPLETED",false,false,true)
 defer with.Close()
 opts.url=with.URL
 if err:=verify(context.Background(),with.Client(),opts,&bytes.Buffer{});err!=nil {
  t.Fatalf("real-event-shaped fixture must pass strict transport validation: %v",err)
 }
}
