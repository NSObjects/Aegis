package httpapi

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "Aegis/internal/application"
)

// brokenRuns reports the run store as unavailable for link resolution.
type brokenRuns struct{}
func (brokenRuns) Submit(context.Context,application.ResearchRequest)(application.RunRecord,error){return application.RunRecord{},nil}
func (brokenRuns) Get(context.Context,string)(application.RunRecord,error){return application.RunRecord{},application.ErrRunNotFound}
func (brokenRuns) Cancel(context.Context,string)error{return nil}
func (brokenRuns) List(context.Context)([]application.RunRecord,error){return nil,errors.New("run store unavailable")}

func newWatchlistHandler(t *testing.T)(http.Handler,*application.FileWatchlistStore){
 t.Helper()
 store,err:=application.NewFileWatchlistStore(filepath.Join(t.TempDir(),"watchlist"))
 if err!=nil{t.Fatal(err)}
 return &Handler{localDev:true,watchlist:store},store
}

func TestWatchlistHTTPLifecycle(t *testing.T){
 handler,store:=newWatchlistHandler(t)
 handler.(*Handler).runs=&fakeRuns{listRecords:[]application.RunRecord{
  {ID:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",Symbol:"600519.SH",State:application.RunCompleted,Mode:application.ModeDecision,CreatedAt:time.Now().UTC().Add(-time.Hour)},
  {ID:"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",Symbol:"600519.SH",State:application.RunFailed,Mode:application.ModeDecision,CreatedAt:time.Now().UTC()},
 }}
 // Empty list first: a fresh watchlist serves an empty array, not null.
 w:=httptest.NewRecorder()
 handler.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/v1/watchlist",nil))
 if w.Code!=http.StatusOK{t.Fatalf("empty list %d",w.Code)}
 if body:=w.Body.String();!strings.Contains(body,`"items":[]`){t.Fatalf("empty items payload %s",body)}
 // Add one entry.
 w=httptest.NewRecorder()
 handler.ServeHTTP(w,httptest.NewRequest(http.MethodPost,"/api/v1/watchlist",strings.NewReader(`{"symbol":"600519.SH","reason":"回调观察","conditions":"跌破 MA20 复查"}`)))
 if w.Code!=http.StatusCreated{t.Fatalf("add %d %s",w.Code,w.Body.String())}
 var created watchlistItemView
 if err:=json.Unmarshal(w.Body.Bytes(),&created);err!=nil||created.Symbol!="600519.SH"||created.Reason!="回调观察"{t.Fatalf("created payload %+v %v",created,err)}
 // List links the most recent run for the watched symbol.
 w=httptest.NewRecorder()
 handler.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/v1/watchlist",nil))
 var listed struct{Items []watchlistItemView `json:"items"`}
 if err:=json.Unmarshal(w.Body.Bytes(),&listed);err!=nil||len(listed.Items)!=1{t.Fatalf("list payload %s %v",w.Body.String(),err)}
 if listed.Items[0].LatestRun==nil||listed.Items[0].LatestRun.RunID!="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"||listed.Items[0].LatestRun.State!=application.RunFailed{
  t.Fatalf("latest run not linked: %+v",listed.Items[0].LatestRun)
 }
 // Symbol without any research links to null, never to another symbol's run.
 if _,err:=store.Add("000001.SZ","reason","");err!=nil{t.Fatal(err)}
 w=httptest.NewRecorder()
 handler.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/v1/watchlist",nil))
 if !strings.Contains(w.Body.String(),`"latest_run":null`){t.Fatalf("unresearched symbol must link null: %s",w.Body.String())}
 // Remove it again.
 w=httptest.NewRecorder()
 handler.ServeHTTP(w,httptest.NewRequest(http.MethodDelete,"/api/v1/watchlist/000001.SZ",nil))
 if w.Code!=http.StatusNoContent{t.Fatalf("delete %d",w.Code)}
 w=httptest.NewRecorder()
 handler.ServeHTTP(w,httptest.NewRequest(http.MethodDelete,"/api/v1/watchlist/000001.SZ",nil))
 if w.Code!=http.StatusNotFound{t.Fatalf("delete missing %d",w.Code)}
}

func TestWatchlistHTTPValidationAndAuth(t *testing.T){
 handler,_:=newWatchlistHandler(t)
 authed:=&Handler{token:strings.Repeat("x",32),watchlist:handler.(*Handler).watchlist}
 req:=httptest.NewRequest(http.MethodGet,"/api/v1/watchlist",nil)
 w:=httptest.NewRecorder();authed.ServeHTTP(w,req)
 if w.Code!=http.StatusUnauthorized{t.Fatalf("without bearer %d",w.Code)}
 req.Header.Set("Authorization","Bearer "+strings.Repeat("x",32))
 w=httptest.NewRecorder();authed.ServeHTTP(w,req)
 if w.Code!=http.StatusOK{t.Fatalf("with bearer %d",w.Code)}
 h:=handler.(*Handler)
 for _,tc:=range []struct{name,body string;want int}{
  {"unknown field",`{"symbol":"600519.SH","reason":"r","extra":1}`,http.StatusBadRequest},
  {"invalid symbol",`{"symbol":"600519","reason":"r"}`,http.StatusBadRequest},
  {"empty reason",`{"symbol":"600519.SH","reason":""}`,http.StatusBadRequest},
  {"trailing json",`{"symbol":"600519.SH","reason":"r"} {}`,http.StatusBadRequest},
 }{
  w:=httptest.NewRecorder()
  h.ServeHTTP(w,httptest.NewRequest(http.MethodPost,"/api/v1/watchlist",strings.NewReader(tc.body)))
  if w.Code!=tc.want{t.Errorf("%s status=%d expected=%d",tc.name,w.Code,tc.want)}
 }
 w=httptest.NewRecorder()
 h.ServeHTTP(w,httptest.NewRequest(http.MethodPost,"/api/v1/watchlist",strings.NewReader(`{"symbol":"600519.SH","reason":"r"}`)))
 if w.Code!=http.StatusCreated{t.Fatalf("valid add %d",w.Code)}
 w=httptest.NewRecorder()
 h.ServeHTTP(w,httptest.NewRequest(http.MethodPost,"/api/v1/watchlist",strings.NewReader(`{"symbol":"600519.SH","reason":"r"}`)))
 if w.Code!=http.StatusConflict{t.Fatalf("duplicate add %d",w.Code)}
 w=httptest.NewRecorder()
 h.ServeHTTP(w,httptest.NewRequest(http.MethodPut,"/api/v1/watchlist",nil))
 if w.Code!=http.StatusMethodNotAllowed{t.Fatalf("method %d",w.Code)}
 w=httptest.NewRecorder()
 h.ServeHTTP(w,httptest.NewRequest(http.MethodDelete,"/api/v1/watchlist/not-a-symbol",nil))
 if w.Code!=http.StatusNotFound{t.Fatalf("bad symbol delete %d",w.Code)}
}

func TestWatchlistStoreFailuresMapToHTTP(t *testing.T){
 store,err:=application.NewFileWatchlistStore(filepath.Join(t.TempDir(),"watchlist"))
 if err!=nil{t.Fatal(err)}
 // Unwired watchlist reports service unavailable, matching scans.
 h:=&Handler{localDev:true}
 w:=httptest.NewRecorder()
 h.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/v1/watchlist",nil))
 if w.Code!=http.StatusServiceUnavailable{t.Fatalf("unwired %d",w.Code)}
 // A failing store maps to 500 on list.
 h.watchlist=&brokenWatchlist{}
 w=httptest.NewRecorder()
 h.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/v1/watchlist",nil))
 if w.Code!=http.StatusInternalServerError{t.Fatalf("broken store %d",w.Code)}
 // Runs-store failure must fail the listing instead of dropping links.
 h2:=&Handler{localDev:true,watchlist:store,runs:brokenRuns{}}
 if _,err:=store.Add("600519.SH","r","");err!=nil{t.Fatal(err)}
 w=httptest.NewRecorder()
 h2.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/v1/watchlist",nil))
 if w.Code!=http.StatusInternalServerError{t.Fatalf("runs failure must fail listing %d",w.Code)}
}

type brokenWatchlist struct{}
func (brokenWatchlist) Add(string,string,string)(application.WatchItem,error){return application.WatchItem{},errors.New("broken")}
func (brokenWatchlist) Remove(string)error{return errors.New("broken")}
func (brokenWatchlist) List()([]application.WatchItem,error){return nil,errors.New("broken")}
