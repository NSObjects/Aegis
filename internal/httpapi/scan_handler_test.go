package httpapi

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "Aegis/internal/application"
 "Aegis/internal/capability/technical"
 "Aegis/internal/strategy/pullback"
)

type fakeScans struct{ last application.ScanRecord; snapshot technical.HistoricalObservation; snapErr error }
func (f *fakeScans) Submit(_ context.Context,asOf,source string,symbols []string)(application.ScanRecord,error){
 f.last=application.ScanRecord{ID:"0123456789abcdef0123456789abcdef",AsOf:asOf,UniverseSource:source,Symbols:symbols,State:application.ScanPending,StrategyVersion:pullback.StrategyVersion,CreatedAt:time.Now().UTC(),UpdatedAt:time.Now().UTC()}
 return f.last,nil
}
func (f *fakeScans) Get(id string)(application.ScanRecord,error){return f.last,nil}
func (f *fakeScans) List(limit int)([]application.ScanRecord,error){return []application.ScanRecord{f.last},nil}
func (f *fakeScans) GetSnapshot(_,symbol string)(technical.HistoricalObservation,error){
 if f.snapErr!=nil{return technical.HistoricalObservation{},f.snapErr}
 return f.snapshot,nil
}
func TestScanHTTPRoutesWithAuthAndInputValidation(t *testing.T){
 fake:=&fakeScans{}
 h:=&Handler{token:strings.Repeat("x",32),scans:fake}
 req:=httptest.NewRequest("POST","/api/v1/scans",strings.NewReader(`{"as_of":"20261022","universe_source":"USER_SUPPLIED","symbols":["600519.SH"]}`))
 w:=httptest.NewRecorder();h.ServeHTTP(w,req)
 if w.Code!=http.StatusUnauthorized{t.Fatalf("without bearer: %d",w.Code)}
 req=httptest.NewRequest("POST","/api/v1/scans",strings.NewReader(`{"as_of":"20261022","universe_source":"USER_SUPPLIED","symbols":["600519.SH"]}`))
 req.Header.Set("Authorization","Bearer "+strings.Repeat("x",32))
 w=httptest.NewRecorder();h.ServeHTTP(w,req)
 if w.Code!=http.StatusAccepted{t.Fatalf("submit %d %s",w.Code,w.Body.String())}
 var rec application.ScanRecord
 if err:=json.Unmarshal(w.Body.Bytes(),&rec);err!=nil||rec.ID==""{t.Fatalf("invalid response %v",err)}
 req=httptest.NewRequest("GET","/api/v1/scans/"+rec.ID,nil)
 req.Header.Set("Authorization","Bearer "+strings.Repeat("x",32))
 w=httptest.NewRecorder();h.ServeHTTP(w,req)
 if w.Code!=http.StatusOK{t.Fatalf("get %d",w.Code)}
 req=httptest.NewRequest("POST","/api/v1/scans",strings.NewReader(`{"as_of":"20261022","universe_source":"USER_SUPPLIED","symbols":["600519.SH"],"unexpected":true}`))
 req.Header.Set("Authorization","Bearer "+strings.Repeat("x",32))
 w=httptest.NewRecorder();h.ServeHTTP(w,req)
 if w.Code!=http.StatusBadRequest{t.Fatalf("unknown field accepted %d",w.Code)}
}

func TestScanHistoryQuery(t *testing.T){
 h:=&Handler{localDev:true,scans:&fakeScans{}}
 for _,tc:=range []struct{path string;want int}{
  {"/api/v1/scans",http.StatusOK},
  {"/api/v1/scans?limit=1",http.StatusOK},
  {"/api/v1/scans?limit=0",http.StatusBadRequest},
  {"/api/v1/scans?limit=101",http.StatusBadRequest},
  {"/api/v1/scans?limit=bogus",http.StatusBadRequest},
 }{
  w:=httptest.NewRecorder()
  h.ServeHTTP(w,httptest.NewRequest(http.MethodGet,tc.path,nil))
  if w.Code!=tc.want{t.Errorf("%s status=%d expected=%d",tc.path,w.Code,tc.want)}
 }
}

func TestScanSnapshotEndpoint(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 base:=technical.HistoricalObservation{Symbol:"600519.SH",RequestedAsOf:"20261022",EffectiveAsOf:"20261022",PriceBasis:"RAW",Bars:[]technical.HistoricalBar{{TradeDate:"20261022",Open:10,High:11,Low:9,Close:10,Volume:100,Amount:1000}}}
 cases:=[]struct{name string;fake *fakeScans;want int}{
  {"serves observation",&fakeScans{snapshot:base},http.StatusOK},
  {"missing symbol snapshot",&fakeScans{snapErr:application.ErrScanSnapshotNotFound},http.StatusNotFound},
  {"unfinished scan",&fakeScans{snapErr:application.ErrScanNotCompleted},http.StatusConflict},
  {"store miss",&fakeScans{snapErr:os.ErrNotExist},http.StatusNotFound},
 }
 for _,tc:=range cases{
  h:=&Handler{localDev:true,scans:tc.fake}
  w:=httptest.NewRecorder()
  h.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/v1/scans/"+id+"/results/600519.SH/snapshot",nil))
  if w.Code!=tc.want{t.Errorf("%s status=%d expected=%d",tc.name,w.Code,tc.want)}
 }
 served:=&fakeScans{snapshot:base}
 h:=&Handler{localDev:true,scans:served}
 w:=httptest.NewRecorder()
 h.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/v1/scans/"+id+"/results/600519.SH/snapshot",nil))
 var got technical.HistoricalObservation
 if err:=json.Unmarshal(w.Body.Bytes(),&got);err!=nil||got.Symbol!="600519.SH"||len(got.Bars)!=1{t.Fatalf("snapshot payload %+v %v",got,err)}
 if w:=func()int{rec:=httptest.NewRecorder();h.ServeHTTP(rec,httptest.NewRequest(http.MethodGet,"/api/v1/scans/not-an-id/results/600519.SH/snapshot",nil));return rec.Code}();w!=http.StatusNotFound{t.Fatalf("bad scan id status=%d",w)}
}
