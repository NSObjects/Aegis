package httpapi

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "Aegis/internal/application"
 "Aegis/internal/strategy/pullback"
)

type fakeScans struct{ last application.ScanRecord }
func (f *fakeScans) Submit(_ context.Context,asOf,source string,symbols []string)(application.ScanRecord,error){
 f.last=application.ScanRecord{ID:"0123456789abcdef0123456789abcdef",AsOf:asOf,UniverseSource:source,Symbols:symbols,State:application.ScanPending,StrategyVersion:pullback.StrategyVersion,CreatedAt:time.Now().UTC(),UpdatedAt:time.Now().UTC()}
 return f.last,nil
}
func (f *fakeScans) Get(id string)(application.ScanRecord,error){return f.last,nil}
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
