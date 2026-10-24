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
 "Aegis/internal/capability/technical"
)

type unavailableScanResearcher struct {}
func (unavailableScanResearcher) Research(ctx context.Context, req technical.Request)(map[string]any,error){
 return map[string]any{"status":"NO_DATA"},nil
}

func TestScanHTTPSubmitCompleteListAfterStoreReopen(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"scans")
 store,err:=application.NewFileScanStore(dir);if err!=nil{t.Fatal(err)}
 manager,err:=application.NewScanManager(store,unavailableScanResearcher{});if err!=nil{t.Fatal(err)}
 handler:=(&Handler{localDev:true}).WithScanController(manager)
 server:=httptest.NewServer(handler);defer server.Close()
 client:=server.Client()
 post,err:=client.Post(server.URL+"/api/v1/scans","application/json",strings.NewReader(`{"as_of":"20261022","universe_source":"USER_SUPPLIED","symbols":["600519.SH","000001.SZ"]}`))
 if err!=nil{t.Fatal(err)}
 defer post.Body.Close()
 if post.StatusCode!=http.StatusAccepted{t.Fatalf("submit code %d",post.StatusCode)}
 var submitted application.ScanRecord
 if err:=json.NewDecoder(post.Body).Decode(&submitted);err!=nil{t.Fatal(err)}
 if submitted.State!=application.ScanPending||submitted.StrategyVersion==""{t.Fatalf("unexpected submitted record %+v",submitted)}
 deadline:=time.Now().Add(3*time.Second)
 var final application.ScanRecord
 for {
  res,err:=client.Get(server.URL+"/api/v1/scans/"+submitted.ID);if err!=nil{t.Fatal(err)}
  err=json.NewDecoder(res.Body).Decode(&final);res.Body.Close()
  if err!=nil{t.Fatal(err)}
  if final.State==application.ScanCompleted||final.State==application.ScanFailed{break}
  if time.Now().After(deadline){t.Fatalf("scan did not complete: %+v",final)}
  time.Sleep(10*time.Millisecond)
 }
 if final.State!=application.ScanCompleted||len(final.Results)!=2{
  t.Fatalf("unexpected terminal state %+v",final)
 }
 for _,result:=range final.Results{
  if string(result.Verdict)!="INSUFFICIENT_DATA"||result.Reason==""{t.Fatalf("provider missingness was not fail-closed: %+v",result)}
 }
 if final.Results[0].Symbol!="000001.SZ"||final.Results[1].Symbol!="600519.SH"{t.Fatalf("not sorted %+v",final.Results)}
 list,err:=client.Get(server.URL+"/api/v1/scans?limit=10");if err!=nil{t.Fatal(err)}
 defer list.Body.Close()
 if list.StatusCode!=http.StatusOK{t.Fatalf("history status %d",list.StatusCode)}
 var history struct{Scans []application.ScanRecord `json:"scans"`}
 if err:=json.NewDecoder(list.Body).Decode(&history);err!=nil{t.Fatal(err)}
 if len(history.Scans)!=1||history.Scans[0].ID!=submitted.ID{t.Fatalf("missing scan history %+v",history)}
 reopened,err:=application.NewFileScanStore(dir);if err!=nil{t.Fatal(err)}
 persisted,err:=reopened.Get(submitted.ID)
 if err!=nil||persisted.State!=application.ScanCompleted||len(persisted.Results)!=2{t.Fatalf("bad durable replay %+v %v",persisted,err)}
}
