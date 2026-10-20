package main

import (
 "bytes"
 "encoding/json"
 "fmt"
 "net"
 "net/http"
 "os"
 "os/exec"
 "syscall"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

// TestRealServerProcessRestart uses the actual cmd/aegis-web binary, HTTP API
// and disk-backed RunStore. Invalid model config causes a deterministic
// pre-agent failure; no provider calls occur.
func TestRealServerProcessRestart(t *testing.T) {
 if testing.Short() { t.Skip("real process integration") }
 dir:=t.TempDir()
 binary:=filepath.Join(dir,"aegis-web")
 build:=exec.Command("go","build","-o",binary,".")
 if out,err:=build.CombinedOutput();err!=nil {t.Fatalf("build server: %v: %s",err,out)}
 port,err:=net.Listen("tcp","127.0.0.1:0");if err!=nil{t.Fatal(err)}
 addr:=port.Addr().String();_ =port.Close()
 root:="http://"+addr+"/api/v1/research/runs"
 client:=&http.Client{Timeout:2*time.Second}
 start:=func()(*exec.Cmd,*bytes.Buffer){
  t.Helper()
  cmd:=exec.Command(binary,"-dev","-listen",addr,"-store",filepath.Join(dir,"runs"),
   "-credential-store",filepath.Join(dir,"credentials.enc"),
   "-evidence-store",filepath.Join(dir,"evidence"),
   "-config",filepath.Join(dir,"missing-model-config.yaml"))
  // Explicitly remove any runner-supplied provider or auth environment.
  for _,env:=range os.Environ(){
   key:=strings.SplitN(env,"=",2)[0]
   if strings.HasPrefix(key,"DEEPSEEK_")||strings.HasPrefix(key,"TUSHARE_")||
     strings.HasPrefix(key,"TAVILY_")||strings.HasPrefix(key,"AEGIS_WEB_")||
     key=="AEGIS_CREDENTIAL_MASTER_KEY"{continue}
   cmd.Env=append(cmd.Env,env)
  }
  logs:=new(bytes.Buffer);cmd.Stdout=logs;cmd.Stderr=logs
  if err:=cmd.Start();err!=nil{t.Fatal(err)}
  deadline:=time.Now().Add(10*time.Second)
  for time.Now().Before(deadline){
   resp,e:=client.Get(root);if e==nil {resp.Body.Close();if resp.StatusCode==200{return cmd,logs}}
   time.Sleep(40*time.Millisecond)
  }
  _=cmd.Process.Kill();_ =cmd.Wait()
  t.Fatalf("real HTTP server failed to start: %s",logs.String())
  return nil,nil
 }
 stop:=func(cmd *exec.Cmd){
  t.Helper()
  _=cmd.Process.Kill()
  _=cmd.Wait()
 }
 first,firstLogs:=start()
 payload:=strings.NewReader(`{"symbol":"600519.SH","mode":"PORTFOLIO_REVIEW","portfolio":{"state":"NOT_PROVIDED"}}`)
 response,err:=client.Post(root,"application/json",payload)
 if err!=nil {stop(first);t.Fatal(err)}
 var created struct{RunID string `json:"run_id"`}
 if err:=json.NewDecoder(response.Body).Decode(&created);err!=nil{response.Body.Close();stop(first);t.Fatal(err)}
 response.Body.Close()
 if response.StatusCode!=202||len(created.RunID)!=32{stop(first);t.Fatalf("submit status %d run %q",response.StatusCode,created.RunID)}
 statusURL:=root+"/"+created.RunID
 var before map[string]any
 var lastState string
 deadline:=time.Now().Add(15*time.Second)
 for time.Now().Before(deadline) {
  resp,e:=client.Get(statusURL)
  if e==nil {
   var data map[string]any
   e=json.NewDecoder(resp.Body).Decode(&data);resp.Body.Close()
   if e==nil {lastState=fmt.Sprint(data["state"]); if data["state"]=="FAILED" {before=data;break}}
  }
  time.Sleep(80*time.Millisecond)
 }
 if before==nil{stop(first);t.Fatalf("expected invalid-config run to fail before restart; last_state=%q; logs=%s",lastState,firstLogs.String())}
 if before["failure_category"]!="RESEARCH_SERVICE_INIT" {stop(first);t.Fatalf("expected classified service initialization failure; got category=%v",before["failure_category"])}
 stop(first)
 second,logs:=start()
 resp,err:=client.Get(statusURL);if err!=nil{t.Fatal(err)}
 var restored map[string]any
 err=json.NewDecoder(resp.Body).Decode(&restored);resp.Body.Close()
 if err!=nil||resp.StatusCode!=200 {t.Fatalf("restored GET: %d %v; server: %s",resp.StatusCode,err,logs.String())}
 for _,key:=range []string{"run_id","state","symbol","failure_code","requested_as_of"}{
  if restored[key]!=before[key]{t.Errorf("restart changed %s: %v -> %v",key,before[key],restored[key])}
 }
 if restored["state"]!="FAILED"{t.Fatalf("failed Run resurrected: %v",restored["state"])}
 artifact,err:=client.Get(statusURL+"/artifact");if err!=nil{t.Fatal(err)}
 artifact.Body.Close()
 if artifact.StatusCode==200{t.Fatal("failed Run leaked artifact after process restart")}
 history,err:=client.Get(root);if err!=nil{t.Fatal(err)}
 var listed map[string]any
 err=json.NewDecoder(history.Body).Decode(&listed);history.Body.Close()
 if err!=nil||history.StatusCode!=200 {t.Fatalf("history after restart: %d %v",history.StatusCode,err)}
 if !strings.Contains(fmt.Sprint(listed),created.RunID){t.Fatalf("persisted Run absent from history: %s",created.RunID)}
 // The second process was started with the same disk store but a new manager.
 // Exercise real SIGTERM shutdown, rather than terminating the server only
 // with SIGKILL. This validates the normal cmd/aegis-web close path; it does
 // not simulate a provider that ignores cancellation.
 if err := second.Process.Signal(syscall.SIGTERM); err != nil {
  t.Fatalf("send SIGTERM to actual Go server: %v", err)
 }
 stopped := make(chan error, 1)
 go func() { stopped <- second.Wait() }()
 select {
 case err := <-stopped:
  if err != nil { t.Fatalf("Go server did not exit cleanly after SIGTERM: %v; logs: %s", err, logs.String()) }
 case <-time.After(13*time.Second):
  _ = second.Process.Kill()
  <-stopped
  t.Fatalf("Go server did not complete graceful shutdown within bounded budget; logs: %s", logs.String())
 }
}
