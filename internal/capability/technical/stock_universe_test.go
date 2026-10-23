package technical

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"
)

func TestListStockUniversePITAndPartitions(t *testing.T) {
 srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  var req tushareRequest
  if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil{t.Error(err);return}
  status:=str(req.Params["list_status"])
  var rows [][]any
  switch status {
  case "L": rows=[][]any{{"600519.SH","20010827","",status},{"300001.SZ","20000101","",status},{"000001.SZ","20270101","",status}}
  case "D": rows=[][]any{{"000002.SZ","19910101","20261231",status},{"600000.SH","19990101","20260101",status}}
  case "P": rows=[][]any{}
  default:t.Errorf("unexpected status %q",status)
  }
  _=json.NewEncoder(w).Encode(map[string]any{"code":0,"data":map[string]any{"fields":[]string{"ts_code","list_date","delist_date","list_status"},"items":rows}})
 }))
 defer srv.Close()
 client,_:=NewTushareClient("test")
 client.endpoint=srv.URL
 got,err:=client.ListStockUniverse(context.Background(),"20261022")
 if err!=nil{t.Fatal(err)}
 want:=[]string{"000002.SZ","600519.SH"}
 if len(got.Symbols)!=len(want){t.Fatalf("got %+v",got)}
 for i,s:=range want{if got.Symbols[i]!=s{t.Fatalf("wrong PIT membership: %+v",got)}}
 if _,err:=client.ListStockUniverse(context.Background(),"20261377");err==nil{t.Fatal("invalid cutoff accepted")}
}
