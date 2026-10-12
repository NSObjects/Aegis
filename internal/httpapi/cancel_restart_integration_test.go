package httpapi

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "Aegis/internal/application"
)

// cancellationExecutor blocks until the genuine RunManager cancels its context.
// It cannot produce a successful report and never calls external providers.
type cancellationExecutor struct{ started chan struct{} }
func (e *cancellationExecutor) Run(ctx context.Context, _ application.ResearchRequest) (*application.ResearchResponse, error) {
 select { case e.started <- struct{}{}: default: }
 <-ctx.Done()
 return nil, ctx.Err()
}

func TestRealHTTPRunningCancellationPersistsAcrossManagerRestart(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"runs")
 store,err:=application.NewFileRunStore(dir);if err!=nil{t.Fatal(err)}
 executor:=&cancellationExecutor{started:make(chan struct{},1)}
 manager,err:=application.NewRunManager(executor,store,1,2);if err!=nil{t.Fatal(err)}
 handler,err:=NewHandler(manager,testToken,application.ModeDecision,7*24*time.Hour)
 if err!=nil{manager.Close();t.Fatal(err)}
 server:=httptest.NewServer(handler)
 defer server.Close()
 client:=&http.Client{Timeout:5*time.Second}
 request:=func(method,path,body string)*http.Response{
  t.Helper()
  req,err:=http.NewRequest(method,server.URL+path,strings.NewReader(body));if err!=nil{t.Fatal(err)}
  req.Header.Set("Authorization","Bearer "+testToken)
  if method=="POST"{req.Header.Set("Content-Type","application/json")}
  response,err:=client.Do(req);if err!=nil{t.Fatal(err)}
  return response
 }
 submitted:=request("POST","/api/v1/research/runs",`{"symbol":"600519.SH"}`)
 var created struct{RunID string `json:"run_id"`}
 if err:=json.NewDecoder(submitted.Body).Decode(&created);err!=nil{t.Fatal(err)}
 submitted.Body.Close()
 if submitted.StatusCode!=http.StatusAccepted||len(created.RunID)!=32{t.Fatalf("submit: %d %#v",submitted.StatusCode,created)}
 select{case <-executor.started:case <-time.After(3*time.Second):t.Fatal("executor did not start")}
 cancel:=request("POST","/api/v1/research/runs/"+created.RunID+"/cancel","")
 cancel.Body.Close()
 if cancel.StatusCode!=http.StatusAccepted{t.Fatalf("cancel HTTP %d",cancel.StatusCode)}
 deadline:=time.Now().Add(4*time.Second)
 var cancelled application.RunRecord
 for time.Now().Before(deadline){
  current,err:=manager.Get(context.Background(),created.RunID)
  if err!=nil{t.Fatal(err)}
  if current.State==application.RunCancelled{cancelled=current;break}
  time.Sleep(10*time.Millisecond)
 }
 if cancelled.State!=application.RunCancelled||len(cancelled.Artifact)!=0{t.Fatalf("cancel did not fail closed: %#v",cancelled)}
 artifact:=request("GET","/api/v1/research/runs/"+created.RunID+"/artifact","")
 artifact.Body.Close()
 if artifact.StatusCode!=http.StatusConflict{t.Fatalf("cancelled artifact status %d",artifact.StatusCode)}
 manager.Close()
 server.Close()
 reopened,err:=application.NewFileRunStore(dir);if err!=nil{t.Fatal(err)}
 restarted,err:=application.NewRunManager(&cancellationExecutor{started:make(chan struct{},1)},reopened,1,2)
 if err!=nil{t.Fatal(err)}
 defer restarted.Close()
 recovered,err:=restarted.Get(context.Background(),created.RunID)
 if err!=nil{t.Fatal(err)}
 if recovered.State!=application.RunCancelled||recovered.ID!=created.RunID||len(recovered.Artifact)!=0 {
  t.Fatalf("cancel state was not preserved across restart: %#v",recovered)
 }
 records,err:=restarted.List(context.Background())
 if err!=nil||len(records)!=1||records[0].ID!=created.RunID||records[0].State!=application.RunCancelled{
  t.Fatalf("cancelled record missing from restarted history: %#v %v",records,err)
 }
}
