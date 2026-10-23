package technical

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"
 "time"
)

// End-to-end deterministic provider smoke: Tushare HTTP contract → raw
// OHLCV + dated adj_factor → validated HistoricalObservation.
func TestHTTPResearcherPITObservationRoundTrip(t *testing.T) {
 cutoff:="20261022"
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  var req tushareRequest
  if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil{t.Error(err);return}
  fields:=[]string{}
  rows:=[][]any{}
  switch req.APIName {
  case "stock_basic":
   fields=[]string{"ts_code","list_status","list_date","delist_date"}
   rows=[][]any{{"600519.SH","L","20010827",""}}
  case "trade_cal":
   fields=[]string{"is_open","pretrade_date"}
   rows=[][]any{{1,"20261021"}}
  case "daily":
   if _,target:=req.Params["trade_date"];target {
    fields=[]string{"ts_code","trade_date"};rows=[][]any{{"600519.SH",cutoff}}
   } else {
    fields=[]string{"ts_code","trade_date","open","high","low","close","pre_close","pct_chg","vol","amount"}
    for i:=0;i<120;i++{
     date:=time.Date(2026,10,22,0,0,0,0,time.UTC).AddDate(0,0,-i).Format("20060102")
     rows=append(rows,[]any{"600519.SH",date,100.0,102.0,99.0,101.0,100.0,1.0,1000.0,10000.0})
    }
   }
  case "adj_factor":
   fields=[]string{"trade_date","adj_factor"}
   for i:=0;i<120;i++{
    date:=time.Date(2026,10,22,0,0,0,0,time.UTC).AddDate(0,0,-i).Format("20060102")
    rows=append(rows,[]any{date,1.0+float64(i)*0.001})
   }
  case "daily_basic":
   fields=[]string{"turnover_rate"};rows=[][]any{{1.2}}
  default:
   t.Errorf("unexpected Tushare api %s",req.APIName)
  }
  _=json.NewEncoder(w).Encode(map[string]any{"code":0,"data":map[string]any{"fields":fields,"items":rows}})
 }))
 defer srv.Close()
 client,err:=NewTushareClient("test-token")
 if err!=nil{t.Fatal(err)}
 client.endpoint=srv.URL
 observer,found,err:=ObserveHistorical(context.Background(),&HTTPResearcher{api:client},Request{Symbol:"600519.SH",AsOf:cutoff})
 if err!=nil{t.Fatal(err)}
 if !found||len(observer.Bars)!=120||len(observer.PITFactors)!=120 {t.Fatalf("incomplete PIT observation: found=%v bars=%d factors=%d",found,len(observer.Bars),len(observer.PITFactors))}
 if observer.EffectiveAsOf!=cutoff||observer.Bars[119].TradeDate!=cutoff||observer.PITFactors[119].TradeDate!=cutoff {t.Fatalf("wrong PIT cutoff: %+v",observer)}
 if observer.PITFactors[0].Factor<=observer.PITFactors[119].Factor {t.Fatal("historical adj factors were lost")}
}
