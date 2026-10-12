// Command aegis-live-check runs ONE paid live research request through the
// existing Aegis HTTP/Eino service. It does not submit trades or mock results.
package main

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "os"
 "regexp"
 "strconv"
 "strings"
 "time"
)

type options struct {url, symbol, mode, token string; requireToolEvents bool}
var runID = regexp.MustCompile("^[0-9a-f]{32}$")
var symbolID = regexp.MustCompile("^[0-9]{6}\\.(SH|SZ)$")

func main() {
 var o options
 var deadline time.Duration
 flag.StringVar(&o.url,"url","http://127.0.0.1:8686","running local Go research API")
 flag.StringVar(&o.symbol,"symbol","600519.SH","A-share stock code")
 flag.StringVar(&o.mode,"mode","PORTFOLIO_REVIEW","research mode")
 flag.DurationVar(&deadline,"timeout",25*time.Minute,"maximum time for one live run")
 flag.BoolVar(&o.requireToolEvents,"require-tool-events",false,"require observed real Technical and Event tool lifecycle events")
 flag.Parse()
 if deadline<=0 {fmt.Fprintln(os.Stderr,"timeout must be positive");os.Exit(2)}
 o.token=os.Getenv("AEGIS_WEB_API_TOKEN")
 ctx,cancel:=context.WithTimeout(context.Background(),deadline);defer cancel()
 if err:=verify(ctx,&http.Client{},o,os.Stdout);err!=nil {
  fmt.Fprintln(os.Stderr,"Live research FAILED:",err);os.Exit(1)
 }
}
func request(ctx context.Context, method,addr,token string,body io.Reader)(*http.Request,error){
 r,err:=http.NewRequestWithContext(ctx,method,addr,body)
 if err!=nil{return nil,err}
 if token!=""{r.Header.Set("Authorization","Bearer "+token)}
 return r,nil
}
func getJSON(c *http.Client,r *http.Request,status int)(map[string]any,error){
 resp,err:=c.Do(r);if err!=nil{return nil,err}
 defer resp.Body.Close()
 if resp.StatusCode!=status {
  // Do not print arbitrary upstream responses containing provider output.
  return nil,fmt.Errorf("HTTP %d (expected %d)",resp.StatusCode,status)
 }
 var data map[string]any
 dec:=json.NewDecoder(io.LimitReader(resp.Body,66<<20))
 if err:=dec.Decode(&data);err!=nil{return nil,err}
 var more any
 if err:=dec.Decode(&more);err!=io.EOF{return nil,errors.New("trailing response data")}
 return data,nil
}
func getText(o map[string]any,k string)string {s,_:=o[k].(string);return s}
func getMap(o map[string]any,k string)map[string]any {m,_:=o[k].(map[string]any);return m}
func verify(ctx context.Context,c *http.Client,o options,out io.Writer)error{
 u,err:=url.Parse(strings.TrimRight(o.url,"/"))
 if err!=nil||u==nil||u.Scheme!="http"||u.User!=nil||u.Host==""||u.Path!=""||u.RawQuery!=""||u.Fragment!=""{
  return errors.New("API URL must be a local HTTP origin")
 }
 if u.Hostname()!="127.0.0.1"&&u.Hostname()!="localhost"&&u.Hostname()!="::1"{
  return errors.New("this verifier only accepts a local loopback API")
 }
 if !symbolID.MatchString(o.symbol){return errors.New("invalid stock symbol")}
 if o.mode!="DECISION"&&o.mode!="PORTFOLIO_REVIEW"{return errors.New("unsupported research mode")}
 root:=u.String()+"/api/v1/research/runs"
 input:=map[string]any{"symbol":o.symbol,"mode":o.mode}
 if o.mode=="PORTFOLIO_REVIEW"{input["portfolio"]=map[string]string{"state":"NOT_PROVIDED"}}
 data,_:=json.Marshal(input)
 post,err:=request(ctx,"POST",root,o.token,bytes.NewReader(data));if err!=nil{return err}
 post.Header.Set("Content-Type","application/json")
 created,err:=getJSON(c,post,202);if err!=nil{return fmt.Errorf("submit run: %w",err)}
 id:=getText(created,"run_id")
 if !runID.MatchString(id)||getText(created,"state")!="PENDING"||getText(created,"symbol")!=o.symbol||getText(created,"mode")!=o.mode{
  return errors.New("invalid initial run contract")
 }
 fmt.Fprintf(out,"Submitted run %s (%s)\n",id,o.mode)
 sse,err:=request(ctx,"GET",root+"/"+id+"/stream",o.token,nil);if err!=nil{return err}
 stages,err:=readStages(c,sse,out,o.requireToolEvents);if err!=nil{return fmt.Errorf("SSE stages: %w",err)}
 get,err:=request(ctx,"GET",root+"/"+id,o.token,nil);if err!=nil{return err}
 final,err:=getJSON(c,get,200);if err!=nil{return fmt.Errorf("run status: %w",err)}
 if getText(final,"run_id")!=id{return errors.New("run ID mismatch")}
 if getText(final,"state")!="COMPLETED"{
  return fmt.Errorf("research ended as %s (%s)",getText(final,"state"),getText(final,"failure_code"))
 }
 artifactRequest,err:=request(ctx,"GET",root+"/"+id+"/artifact",o.token,nil);if err!=nil{return err}
 artifact,err:=getJSON(c,artifactRequest,200);if err!=nil{return fmt.Errorf("completed artifact: %w",err)}
 if err:=validateArtifact(artifact,o.mode,o.symbol);err!=nil{return fmt.Errorf("invalid artifact: %w",err)}
 required:=[]string{"Market","Technical","Event","Synthesis"}
 if o.mode=="PORTFOLIO_REVIEW"{required=append(required,"Debate","ResearchManager","Trading","Risk","Portfolio","FinalRiskGate")}
 for _,name:=range required{
  if !stages[name]{return fmt.Errorf("missing successful stage: %s",name)}
 }
 fmt.Fprintf(out,"PASS: real ResearchRun %s completed; stage events and typed artifact verified.\n",id)
 return nil
}
func readStages(c *http.Client,r *http.Request,out io.Writer,requireTools bool)(map[string]bool,error){
 response,err:=c.Do(r);if err!=nil{return nil,err}
 defer response.Body.Close()
 if response.StatusCode!=200{return nil,fmt.Errorf("HTTP %d",response.StatusCode)}
 if !strings.Contains(response.Header.Get("Content-Type"),"text/event-stream"){return nil,errors.New("missing SSE content type")}
 scanner:=bufio.NewScanner(response.Body);scanner.Buffer(make([]byte,4096),2<<20)
 done:=map[string]bool{}
 toolsStarted:=map[string]bool{}
 toolsFinished:=map[string]bool{}
 var last,id uint64
 var kind,data string
 terminal:=false
 flush:=func()error{
  if kind==""&&data==""{return nil}
  if kind!="research_stage"||data==""{return errors.New("unexpected SSE event")}
  var ev map[string]any
  if json.Unmarshal([]byte(data),&ev)!=nil{return errors.New("invalid stage JSON")}
  seq,ok:=ev["seq"].(float64)
  if !ok||uint64(seq)!=id||id!=last+1||seq<1||seq!=float64(uint64(seq)){return errors.New("stage SSE sequence mismatch")}
  name,status:=getText(ev,"stage"),getText(ev,"status")
  if name==""||status==""||len(name)>100{return errors.New("invalid stage metadata")}
  if getText(ev,"kind")=="TOOL" {
   toolName:=getText(ev,"tool")
   if toolName=="" || len(toolName)>64 || !regexp.MustCompile(`^[A-Za-z0-9_-]+// Command aegis-live-check runs ONE paid live research request through the
// existing Aegis HTTP/Eino service. It does not submit trades or mock results.
package main

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "os"
 "regexp"
 "strconv"
 "strings"
 "time"
)

type options struct {url, symbol, mode, token string; requireToolEvents bool}
var runID = regexp.MustCompile("^[0-9a-f]{32}$")
var symbolID = regexp.MustCompile("^[0-9]{6}\\.(SH|SZ)$")

func main() {
 var o options
 var deadline time.Duration
 flag.StringVar(&o.url,"url","http://127.0.0.1:8686","running local Go research API")
 flag.StringVar(&o.symbol,"symbol","600519.SH","A-share stock code")
 flag.StringVar(&o.mode,"mode","PORTFOLIO_REVIEW","research mode")
 flag.DurationVar(&deadline,"timeout",25*time.Minute,"maximum time for one live run")
 flag.BoolVar(&o.requireToolEvents,"require-tool-events",false,"require observed real Technical and Event tool lifecycle events")
 flag.Parse()
 if deadline<=0 {fmt.Fprintln(os.Stderr,"timeout must be positive");os.Exit(2)}
 o.token=os.Getenv("AEGIS_WEB_API_TOKEN")
 ctx,cancel:=context.WithTimeout(context.Background(),deadline);defer cancel()
 if err:=verify(ctx,&http.Client{},o,os.Stdout);err!=nil {
  fmt.Fprintln(os.Stderr,"Live research FAILED:",err);os.Exit(1)
 }
}
func request(ctx context.Context, method,addr,token string,body io.Reader)(*http.Request,error){
 r,err:=http.NewRequestWithContext(ctx,method,addr,body)
 if err!=nil{return nil,err}
 if token!=""{r.Header.Set("Authorization","Bearer "+token)}
 return r,nil
}
func getJSON(c *http.Client,r *http.Request,status int)(map[string]any,error){
 resp,err:=c.Do(r);if err!=nil{return nil,err}
 defer resp.Body.Close()
 if resp.StatusCode!=status {
  // Do not print arbitrary upstream responses containing provider output.
  return nil,fmt.Errorf("HTTP %d (expected %d)",resp.StatusCode,status)
 }
 var data map[string]any
 dec:=json.NewDecoder(io.LimitReader(resp.Body,66<<20))
 if err:=dec.Decode(&data);err!=nil{return nil,err}
 var more any
 if err:=dec.Decode(&more);err!=io.EOF{return nil,errors.New("trailing response data")}
 return data,nil
}
func getText(o map[string]any,k string)string {s,_:=o[k].(string);return s}
func getMap(o map[string]any,k string)map[string]any {m,_:=o[k].(map[string]any);return m}
func verify(ctx context.Context,c *http.Client,o options,out io.Writer)error{
 u,err:=url.Parse(strings.TrimRight(o.url,"/"))
 if err!=nil||u==nil||u.Scheme!="http"||u.User!=nil||u.Host==""||u.Path!=""||u.RawQuery!=""||u.Fragment!=""{
  return errors.New("API URL must be a local HTTP origin")
 }
 if u.Hostname()!="127.0.0.1"&&u.Hostname()!="localhost"&&u.Hostname()!="::1"{
  return errors.New("this verifier only accepts a local loopback API")
 }
 if !symbolID.MatchString(o.symbol){return errors.New("invalid stock symbol")}
 if o.mode!="DECISION"&&o.mode!="PORTFOLIO_REVIEW"{return errors.New("unsupported research mode")}
 root:=u.String()+"/api/v1/research/runs"
 input:=map[string]any{"symbol":o.symbol,"mode":o.mode}
 if o.mode=="PORTFOLIO_REVIEW"{input["portfolio"]=map[string]string{"state":"NOT_PROVIDED"}}
 data,_:=json.Marshal(input)
 post,err:=request(ctx,"POST",root,o.token,bytes.NewReader(data));if err!=nil{return err}
 post.Header.Set("Content-Type","application/json")
 created,err:=getJSON(c,post,202);if err!=nil{return fmt.Errorf("submit run: %w",err)}
 id:=getText(created,"run_id")
 if !runID.MatchString(id)||getText(created,"state")!="PENDING"||getText(created,"symbol")!=o.symbol||getText(created,"mode")!=o.mode{
  return errors.New("invalid initial run contract")
 }
 fmt.Fprintf(out,"Submitted run %s (%s)\n",id,o.mode)
 sse,err:=request(ctx,"GET",root+"/"+id+"/stream",o.token,nil);if err!=nil{return err}
 stages,err:=readStages(c,sse,out,o.requireToolEvents);if err!=nil{return fmt.Errorf("SSE stages: %w",err)}
 get,err:=request(ctx,"GET",root+"/"+id,o.token,nil);if err!=nil{return err}
 final,err:=getJSON(c,get,200);if err!=nil{return fmt.Errorf("run status: %w",err)}
 if getText(final,"run_id")!=id{return errors.New("run ID mismatch")}
 if getText(final,"state")!="COMPLETED"{
  return fmt.Errorf("research ended as %s (%s)",getText(final,"state"),getText(final,"failure_code"))
 }
 artifactRequest,err:=request(ctx,"GET",root+"/"+id+"/artifact",o.token,nil);if err!=nil{return err}
 artifact,err:=getJSON(c,artifactRequest,200);if err!=nil{return fmt.Errorf("completed artifact: %w",err)}
 if err:=validateArtifact(artifact,o.mode,o.symbol);err!=nil{return fmt.Errorf("invalid artifact: %w",err)}
 required:=[]string{"Market","Technical","Event","Synthesis"}
 if o.mode=="PORTFOLIO_REVIEW"{required=append(required,"Debate","ResearchManager","Trading","Risk","Portfolio","FinalRiskGate")}
 for _,name:=range required{
  if !stages[name]{return fmt.Errorf("missing successful stage: %s",name)}
 }
 fmt.Fprintf(out,"PASS: real ResearchRun %s completed; stage events and typed artifact verified.\n",id)
 return nil
}
func readStages(c *http.Client,r *http.Request,out io.Writer,requireTools bool)(map[string]bool,error){
 response,err:=c.Do(r);if err!=nil{return nil,err}
 defer response.Body.Close()
 if response.StatusCode!=200{return nil,fmt.Errorf("HTTP %d",response.StatusCode)}
 if !strings.Contains(response.Header.Get("Content-Type"),"text/event-stream"){return nil,errors.New("missing SSE content type")}
 scanner:=bufio.NewScanner(response.Body);scanner.Buffer(make([]byte,4096),2<<20)
 done:=map[string]bool{}
 toolsStarted:=map[string]bool{}
 toolsFinished:=map[string]bool{}
 var last,id uint64
 var kind,data string
 terminal:=false
 flush:=func()error{
  if kind==""&&data==""{return nil}
  if kind!="research_stage"||data==""{return errors.New("unexpected SSE event")}
  var ev map[string]any
  if json.Unmarshal([]byte(data),&ev)!=nil{return errors.New("invalid stage JSON")}
  seq,ok:=ev["seq"].(float64)
  if !ok||uint64(seq)!=id||id!=last+1||seq<1||seq!=float64(uint64(seq)){return errors.New("stage SSE sequence mismatch")}
  name,status:=getText(ev,"stage"),getText(ev,"status")
  if name==""||status==""||len(name)>100{return errors.New("invalid stage metadata")}
).MatchString(toolName) ||
     (name!="Technical" && name!="Event") ||
     (status!="START" && status!="DONE" && status!="FAIL") {
    return errors.New("invalid observed tool metadata")
   }
   key:=name+":"+toolName
   if status=="START" {toolsStarted[key]=true}
   if status=="DONE" || status=="FAIL" {
    if !toolsStarted[key] {return errors.New("tool terminal without observed start")}
    toolsFinished[key]=true
   }
  } else if getText(ev,"kind")!="" || getText(ev,"tool")!="" {
   return errors.New("unexpected tool metadata in stage event")
  }
  last=id
  if getText(ev,"kind")=="" && status=="DONE"{done[name]=true}
  if name=="Run"&&(status=="COMPLETED"||status=="FAILED"||status=="CANCELLED"){terminal=true}
  if getText(ev,"kind")=="TOOL" {
   fmt.Fprintf(out,"[%d] %s TOOL: %s\n",id,name,status) // never print provider name, inputs or response
  } else {fmt.Fprintf(out,"[%d] %s: %s\n",id,name,status)}
  return nil
 }
 for scanner.Scan(){
  line:=scanner.Text()
  if line==""{if err:=flush();err!=nil{return nil,err};kind="";data="";id=0;continue}
  if strings.HasPrefix(line,":"){continue}
  if strings.HasPrefix(line,"id: "){id,err=strconv.ParseUint(strings.TrimPrefix(line,"id: "),10,64);if err!=nil{return nil,errors.New("bad SSE id")}}
  if strings.HasPrefix(line,"event: "){kind=strings.TrimPrefix(line,"event: ")}
  if strings.HasPrefix(line,"data: "){data+=strings.TrimPrefix(line,"data: ")}
 }
 if err:=scanner.Err();err!=nil{return nil,err}
 if kind!=""||data!=""{if err:=flush();err!=nil{return nil,err}}
 if !terminal||last<2{return nil,errors.New("SSE stream stopped before a terminal Run event")}
 if requireTools {
  hasTechnical,hasEvent:=false,false
  for key:=range toolsFinished {
   if strings.HasPrefix(key,"Technical:"){hasTechnical=true}
   if strings.HasPrefix(key,"Event:"){hasEvent=true}
  }
  if !hasTechnical || !hasEvent {return nil,fmt.Errorf("missing observed tool lifecycle: Technical=%t Event=%t",hasTechnical,hasEvent)}
 }
 return done,nil
}
func validateArtifact(a map[string]any,mode,symbol string)error{
 if mode=="DECISION"{
  if getText(a,"symbol")!=symbol{return errors.New("decision symbol mismatch")}
  if getText(a,"market_view")==""{return errors.New("missing market view")}
  switch getText(a,"action"){case "WATCH","AVOID","CANDIDATE":default:return errors.New("invalid research action")}
  evidence,ok:=a["evidence"].([]any)
  if !ok||len(evidence)==0{return errors.New("missing decision evidence")}
  return nil
 }
 research:=getMap(a,"research_artifact")
 if research==nil||getText(research,"symbol")!=symbol{return errors.New("research artifact missing or mismatched")}
 for _,name:=range []string{"market","technical","event","synthesis"}{
  if getMap(research,name)==nil{return fmt.Errorf("missing research domain: %s",name)}
 }
 portfolio:=getMap(a,"portfolio_review")
 decision:=getMap(portfolio,"portfolio_decision")
 if decision==nil||getText(decision,"portfolio_state")!="NOT_PROVIDED"{return errors.New("unknown portfolio state was altered")}
 gate:=getMap(a,"final_risk_gate")
 if gate==nil||getText(gate,"disposition")==""{return errors.New("missing final risk disposition")}
 if getText(gate,"disposition")!=getText(decision,"disposition"){return errors.New("final risk gate contradicts portfolio decision")}
 return nil
}
