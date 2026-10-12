package httpapi

import (
 "encoding/json"
 "net/http"
 "strings"
 "testing"

 technicalcap "Aegis/internal/capability/technical"
)

func TestHTTPCompletedRunExposesSameRunHistoricalOHLCV(t *testing.T) {
 rec:=testRecord()
 rec.HistoricalOHLCV=&technicalcap.HistoricalObservation{
  Symbol:rec.Symbol,RequestedAsOf:"20261010",EffectiveAsOf:"20261009",PriceBasis:"RAW",
  Bars:[]technicalcap.HistoricalBar{
   {TradeDate:"20261008",Open:10,High:12,Low:9,Close:11,Volume:0,Amount:0},
   {TradeDate:"20261009",Open:11,High:13,Low:10,Close:12,Volume:100,Amount:1000},
  },
 }
 h:=mkHandler(t,&fakeRuns{rec:rec},rec.Mode)
 endpoint:="/api/v1/research/runs/"+rec.ID
 unauthorized:=apiRequest(t,h,http.MethodGet,endpoint,"",false,nil)
 if unauthorized.Code!=http.StatusUnauthorized {t.Fatalf("unauthorized response=%d",unauthorized.Code)}
 got:=apiRequest(t,h,http.MethodGet,endpoint,"",true,nil)
 if got.Code!=http.StatusOK {t.Fatalf("run response=%d: %s",got.Code,got.Body)}
 var payload struct{
  RunID string `json:"run_id"`
  Historical struct{
   Symbol string `json:"symbol"`
   RequestedAsOf string `json:"requested_as_of"`
   EffectiveAsOf string `json:"effective_as_of"`
   PriceBasis string `json:"price_basis"`
   Bars []technicalcap.HistoricalBar `json:"bars"`
  } `json:"historical_ohlcv"`
 }
 if err:=json.Unmarshal(got.Body.Bytes(),&payload);err!=nil{t.Fatal(err)}
 if payload.RunID!=rec.ID || payload.Historical.Symbol!=rec.Symbol || payload.Historical.RequestedAsOf!="20261010" || payload.Historical.PriceBasis!="RAW" || len(payload.Historical.Bars)!=2 || payload.Historical.Bars[0].Volume!=0 {t.Fatalf("lost observed history: %#v",payload)}
 artifact:=apiRequest(t,h,http.MethodGet,endpoint+"/artifact","",true,nil)
 if artifact.Code!=http.StatusOK || strings.Contains(artifact.Body.String(),"historical_ohlcv") {t.Fatalf("workflow artifact must remain independent: %d %s",artifact.Code,artifact.Body)}
}
