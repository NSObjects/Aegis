package technical

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"
)

func TestHTTPResearcherMissingDailyIsNoDataNotSuspended(t *testing.T){
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  var q tushareRequest;if err:=json.NewDecoder(r.Body).Decode(&q);err!=nil{t.Fatal(err)}
  fields:=[]string{};items:=[][]any{}
  switch q.APIName {
  case "stock_basic": fields=[]string{"ts_code","list_status","list_date","delist_date"};items=[][]any{{"600519.SH","L","20010827",""}}
  case "trade_cal": fields=[]string{"is_open","pretrade_date"};items=[][]any{{1,"20260929"}}
  case "daily": fields=[]string{"trade_date"}
  default: t.Fatalf("unexpected api %s",q.APIName)
  }
  _=json.NewEncoder(w).Encode(map[string]any{"code":0,"msg":nil,"data":map[string]any{"fields":fields,"items":items}})
 }));defer srv.Close()
 c,_:=NewTushareClient("token");c.endpoint=srv.URL;r:=&HTTPResearcher{api:c}
 got,err:=r.Research(context.Background(),Request{Symbol:"600519.SH",AsOf:"20260930"});if err!=nil{t.Fatal(err)}
 if got["status"]!="NO_DATA"{t.Fatalf("status=%v, want NO_DATA",got["status"])}
 if got["status"]=="SUSPENDED"{t.Fatal("missing daily row must never infer suspension")}
}
func TestIndicators(t *testing.T){
 x:=make([]float64,60);for i:=range x{x[i]=float64(i+1)}
 if ma(x,5)==nil||rsi(x,14)==nil{t.Fatal("expected MA and RSI")}
 m:=macd(x);if m["dif"]==nil||m["dea"]==nil||m["hist"]==nil{t.Fatal("expected MACD")}
}
