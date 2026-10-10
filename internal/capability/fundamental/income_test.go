package fundamental

import(
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"
)
func TestLatestIncomeHonorsAnnouncementCutoff(t *testing.T){
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  var q struct{API string `json:"api_name"`;Params map[string]string `json:"params"`}
  if err:=json.NewDecoder(r.Body).Decode(&q);err!=nil{t.Fatal(err)}
  if q.API!="income"||q.Params["ts_code"]!="600519.SH"{t.Fatalf("bad query: %+v",q)}
  w.Header().Set("Content-Type","application/json")
  _,_=w.Write([]byte(`{"code":0,"data":{"fields":["ts_code","ann_date","end_date","revenue","n_income"],"items":[["600519.SH","20261011","20260930",200,40],["600519.SH","20260830","20260630",100,20],["600519.SH","20260420","20260331",80,10]]}}`))
 }))
 defer server.Close()
 source:=&IncomeSource{Token:"test",Endpoint:server.URL}
 got,err:=source.LatestIncome(context.Background(),"600519.SH","20261010")
 if err!=nil{t.Fatal(err)}
 if got.PeriodEnd!="20260630"||got.Revenue==nil||*got.Revenue!=100{t.Fatalf("unexpected point-in-time report: %+v",got)}
}
func TestLatestIncomeRejectsMissingSource(t *testing.T){
 source:=&IncomeSource{}
 if _,err:=source.LatestIncome(context.Background(),"600519.SH","20261010");err==nil{t.Fatal("missing token must fail")}
 if _,err:=source.LatestIncome(context.Background(),"bad","20261010");err==nil{t.Fatal("bad symbol must fail")}
}
