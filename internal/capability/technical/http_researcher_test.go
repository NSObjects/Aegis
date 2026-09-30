package technical

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestHTTPResearcherFallsBackToLatestAvailableBar(t *testing.T){
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  var q tushareRequest;if err:=json.NewDecoder(r.Body).Decode(&q);err!=nil{t.Fatal(err)}
  fields:=[]string{};items:=[][]any{}
  switch q.APIName {
  case "stock_basic": fields=[]string{"ts_code","list_status","list_date","delist_date"};items=[][]any{{"600519.SH","L","20010827",""}}
  case "trade_cal": fields=[]string{"is_open","pretrade_date"};items=[][]any{{1,"20260929"}}
  case "daily":
   p:=q.Params
   if p["trade_date"]=="20260930" { fields=[]string{"ts_code","trade_date"};break }
   if _,ok:=p["start_date"];ok {
    if q.Fields=="trade_date" {fields=[]string{"trade_date"};items=[][]any{{"20260929"}};break}
    fields=[]string{"ts_code","trade_date","open","high","low","close","pre_close","pct_chg","vol","amount"}
    for i:=1;i<=120;i++ { date:="20260929";items=append(items,[]any{"600519.SH",date,100.0,102.0,99.0,float64(100+i)/2,100.0,1.0,1000.0,10000.0}) }
   }
  case "adj_factor":
   fields=[]string{"trade_date","adj_factor"};items=[][]any{{"20260929",1.0}}
  case "daily_basic": fields=[]string{"turnover_rate"};items=[][]any{{1.2}}
  default:t.Fatalf("unexpected api %s",q.APIName)
  }
  _=json.NewEncoder(w).Encode(map[string]any{"code":0,"msg":nil,"data":map[string]any{"fields":fields,"items":items}})
 }));defer srv.Close()
 c,_:=NewTushareClient("token");c.endpoint=srv.URL;r:=&HTTPResearcher{api:c}
 got,err:=r.Research(context.Background(),Request{Symbol:"600519.SH",AsOf:"20260930"});if err!=nil{t.Fatal(err)}
 if got["status"]!="NORMAL"{t.Fatalf("status=%v, want NORMAL",got["status"])}
 if got["effective_as_of"]!="20260929"{t.Fatalf("effective_as_of=%v",got["effective_as_of"])}
 warnings:=got["warnings"].([]string);if len(warnings)==0||!strings.Contains(warnings[0],"using latest available bar 20260929"){t.Fatalf("warnings=%v",warnings)}
 if got["status"]=="SUSPENDED"{t.Fatal("missing requested-day bar must never infer suspension")}
}

func TestHTTPResearcherNoHistoricalBarIsNoData(t *testing.T){
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  var q tushareRequest;_ = json.NewDecoder(r.Body).Decode(&q);fields:=[]string{};items:=[][]any{}
  switch q.APIName {case "stock_basic":fields=[]string{"ts_code","list_status","list_date","delist_date"};items=[][]any{{"600519.SH","L","20010827",""}};case "trade_cal":fields=[]string{"is_open","pretrade_date"};items=[][]any{{1,"20260929"}};case "daily":fields=[]string{"trade_date"}}
  _=json.NewEncoder(w).Encode(map[string]any{"code":0,"data":map[string]any{"fields":fields,"items":items}})
 }));defer srv.Close()
 c,_:=NewTushareClient("token");c.endpoint=srv.URL;r:=&HTTPResearcher{api:c}
 got,err:=r.Research(context.Background(),Request{Symbol:"600519.SH",AsOf:"20260930"});if err!=nil{t.Fatal(err)}
 if got["status"]!="NO_DATA"{t.Fatalf("status=%v, want NO_DATA",got["status"])}
}

func TestIndicators(t *testing.T){
 x:=make([]float64,60);for i:=range x{x[i]=float64(i+1)}
 if ma(x,5)==nil||rsi(x,14)==nil{t.Fatal("expected MA and RSI")}
 m:=macd(x);if m["dif"]==nil||m["dea"]==nil||m["hist"]==nil{t.Fatal("expected MACD")}
}
