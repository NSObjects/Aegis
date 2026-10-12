package httpapi

import (
 "context"
 "encoding/json"
 "net/http"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "Aegis/internal/application"
 technicalcap "Aegis/internal/capability/technical"
)

// The HTTP Handler needs a RunController; the read paths delegate to the
// freshly reopened real store. Mutating operations are intentionally disabled.
type reopenedHistoryReadController struct { *application.FileRunStore }
func (r reopenedHistoryReadController) Submit(context.Context, application.ResearchRequest) (application.RunRecord,error) {return application.RunRecord{}, application.ErrRunNotActive}
func (r reopenedHistoryReadController) Cancel(context.Context,string) error {return application.ErrRunNotActive}

// Exercise the real file-backed RunStore on both sides of a process-like
// restart, then read the same Run through the production HTTP handler.
func TestHistoricalOHLCVSurvivesStoreReopenThroughHTTP(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"runs")
 first,err:=application.NewFileRunStore(dir)
 if err!=nil{t.Fatal(err)}
 at:=time.Date(2026,10,9,8,0,0,0,time.UTC)
 rec:=application.RunRecord{ID:strings.Repeat("b",32),Mode:application.ModeDecision,Symbol:"600519.SH",RequestedAsOf:at,CreatedAt:at,State:application.RunCompleted,FinishedAt:&at,
  Artifact:json.RawMessage(`{"symbol":"600519.SH","action":"WATCH"}`),
  HistoricalOHLCV:&technicalcap.HistoricalObservation{Symbol:"600519.SH",RequestedAsOf:"20261009",EffectiveAsOf:"20261009",PriceBasis:"RAW",Bars:[]technicalcap.HistoricalBar{
   {TradeDate:"20261008",Open:10,High:12,Low:9,Close:11,Volume:0,Amount:0},
   {TradeDate:"20261009",Open:11,High:13,Low:10,Close:12,Volume:100,Amount:1000},
  }},
 }
 if err:=first.Save(context.Background(),rec);err!=nil{t.Fatal(err)}
 second,err:=application.NewFileRunStore(dir)
 if err!=nil{t.Fatal(err)}
 h,err:=NewHandler(reopenedHistoryReadController{second},testToken,application.ModeDecision,7*24*time.Hour)
 if err!=nil{t.Fatal(err)}
 res:=apiRequest(t,h,http.MethodGet,"/api/v1/research/runs/"+rec.ID,"",true,nil)
 if res.Code!=http.StatusOK{t.Fatalf("reopened HTTP %d: %s",res.Code,res.Body)}
 var got application.RunRecord
 if err:=json.Unmarshal(res.Body.Bytes(),&got);err!=nil{t.Fatal(err)}
 if got.HistoricalOHLCV==nil||got.HistoricalOHLCV.Symbol!=rec.Symbol||got.HistoricalOHLCV.PriceBasis!="RAW"||got.HistoricalOHLCV.RequestedAsOf!="20261009"||len(got.HistoricalOHLCV.Bars)!=2||got.HistoricalOHLCV.Bars[0].Volume!=0{t.Fatalf("reopened history mismatch: %#v",got.HistoricalOHLCV)}
 unauth:=apiRequest(t,h,http.MethodGet,"/api/v1/research/runs/"+rec.ID,"",false,nil)
 if unauth.Code!=http.StatusUnauthorized{t.Fatalf("reopened history leaked to unauthenticated caller: %d",unauth.Code)}
}
