package technical

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"

)

// This fixture has a complete contiguous calendar and 90 trade sessions.
// All tests are offline; no real Tushare credential or provider is used.
type dayFixture struct{
 mu sync.Mutex
 counts map[string]int
 missingFactor string
 missingPrice string
 corruptSymbol bool
}
func (f *dayFixture) serve(w http.ResponseWriter,r *http.Request){
 var q tushareRequest
 if err:=json.NewDecoder(r.Body).Decode(&q);err!=nil {http.Error(w,"invalid request",400);return}
 key:=q.APIName
 f.mu.Lock()
 if f.counts==nil {f.counts=map[string]int{}}
 f.counts[key]++
 f.mu.Unlock()
 fields:=[]string{}
 rows:=[][]any{}
 date:=str(q.Params["trade_date"])
 switch q.APIName {
 case "trade_cal":
  fields=[]string{"cal_date","is_open"}
  from:=str(q.Params["start_date"]);to:=str(q.Params["end_date"])
  start,err:=time.Parse("20060102",from);if err!=nil{panic(err)}
  end,err:=time.Parse("20060102",to);if err!=nil{panic(err)}
  for d:=start;!d.After(end);d=d.AddDate(0,0,1){
   open:=float64(1)
   if d.Weekday()==time.Saturday||d.Weekday()==time.Sunday {open=0}
   rows=append(rows,[]any{d.Format("20060102"),open})
  }
 case "daily":
  fields=[]string{"ts_code","trade_date","open","high","low","close","vol","amount"}
  rows=append(rows,[]any{"600000.SH",date,10.0,11.0,9.0,10.1,1000.0,10000.0})
  if date!=f.missingPrice {
   code:="600519.SH"
   if f.corruptSymbol {code="X"}
   rows=append(rows,[]any{code,date,100.0,102.0,99.0,101.0,900.0,9000.0})
  }
 case "adj_factor":
  fields=[]string{"ts_code","trade_date","adj_factor"}
  rows=append(rows,[]any{"600000.SH",date,1.0})
  if date!=f.missingFactor {rows=append(rows,[]any{"600519.SH",date,1.0})}
 default:
  http.Error(w,"unexpected endpoint",400);return
 }
 _=json.NewEncoder(w).Encode(map[string]any{"code":0,"data":map[string]any{"fields":fields,"items":rows}})
}
func (f *dayFixture) count(api string)int{f.mu.Lock();defer f.mu.Unlock();return f.counts[api]}
func newFakeMarketDay(t *testing.T,f *dayFixture,dir string)(*HTTPResearcher,func()){
 t.Helper()
 srv:=httptest.NewServer(http.HandlerFunc(f.serve))
 cli,err:=NewTushareClient("no-secret");if err!=nil{t.Fatal(err)}
 cli.endpoint=srv.URL
 r:=&HTTPResearcher{api:cli}
 if err:=r.EnableMarketDayCache(dir);err!=nil{t.Fatal(err)}
 r.poolCache.requestInterval=0
 return r,srv.Close
}
func recentDay(asOf string,back int)string {
 d,_:=time.Parse("20060102",asOf)
 for i:=0;i<back;i++{
  d=d.AddDate(0,0,-1)
  for d.Weekday()==time.Saturday||d.Weekday()==time.Sunday {d=d.AddDate(0,0,-1)}
 }
 return d.Format("20060102")
}
func TestMarketDayCachePrewarmReusesSnapshotsAfterRestart(t *testing.T){
 const asOf="20261022"
 dir:=filepath.Join(t.TempDir(),"days")
 fixture:=&dayFixture{}
 r,closeServer:=newFakeMarketDay(t,fixture,dir)
 progress:=0
 cb:=func(done,total int)error{
  if total!=marketDaySessions||done<1||done>total{return errors.New("invalid cache progress")}
  progress=done;return nil
 }
 symbols:=[]string{"600000.SH","600519.SH"}
 if err:=r.PreparePool(context.Background(),asOf,symbols,cb);err!=nil{t.Fatal(err)}
 if progress!=90||fixture.count("daily")!=90||fixture.count("adj_factor")!=90||fixture.count("trade_cal")!=1{
  t.Fatalf("wrong prefetch calls %+v progress=%d",fixture.counts,progress)
 }
 observed,found,err:=ObserveHistorical(WithVerifiedPoolCutoff(context.Background(),asOf),r,Request{Symbol:"600519.SH",AsOf:asOf})
 if err!=nil||!found||len(observed.Bars)!=65||len(observed.PITFactors)!=65||
  observed.EffectiveAsOf!=asOf {t.Fatalf("cached observation found=%v bars=%d factors=%d err=%v",found,len(observed.Bars),len(observed.PITFactors),err)}
 // No provider call is permitted during per-stock evaluation after prefetch.
 if fixture.count("daily")!=90 {t.Fatal("cached scan fetched per-stock daily")}
 closeServer()

 // Reconstruct a new client and researcher using the same private files.
 r2,close2:=newFakeMarketDay(t,fixture,dir);defer close2()
 if err:=r2.PreparePool(context.Background(),asOf,symbols,nil);err!=nil{t.Fatal(err)}
 if fixture.count("daily")!=90||fixture.count("adj_factor")!=90||fixture.count("trade_cal")!=2{
  t.Fatalf("restart did not reuse the exact date snapshots: %+v",fixture.counts)
 }
}
func TestMarketDayCacheMissingPITAndMissingTradeFailClosed(t *testing.T){
 const asOf="20261022"
 for _,tc:=range []struct{name string;factor,price string;wantFound bool;wantReason string}{
  {"missing factor",recentDay(asOf,4),"",true,"MISSING_PIT_ADJUSTMENT"},
  {"missing one daily", "",recentDay(asOf,4),false,""},
 }{
  t.Run(tc.name,func(t *testing.T){
   f:=&dayFixture{missingFactor:tc.factor,missingPrice:tc.price}
   r,done:=newFakeMarketDay(t,f,filepath.Join(t.TempDir(),"cache"));defer done()
   if err:=r.PreparePool(context.Background(),asOf,[]string{"600519.SH"},nil);err!=nil{t.Fatal(err)}
   o,found,err:=ObserveHistorical(WithVerifiedPoolCutoff(context.Background(),asOf),r,Request{Symbol:"600519.SH",AsOf:asOf})
   if err!=nil||found!=tc.wantFound {t.Fatalf("found=%v err=%v",found,err)}
   if found {
    if tc.wantReason!="MISSING_PIT_ADJUSTMENT"||len(o.PITFactors)!=0||len(o.Bars)!=65 {
     t.Fatalf("missing factors should not be invented %+v",o)
    }
   }
  })
 }
}
func TestMarketDayCacheRejectsCapAndTampering(t *testing.T){
 f:=&dayFixture{}
 r,done:=newFakeMarketDay(t,f,filepath.Join(t.TempDir(),"cache"));defer done()
 date:="20261022"
 _,err:=r.poolCache.readDate(context.Background(),"daily",date)
 if err!=nil{t.Fatal(err)}
 path:=filepath.Join(r.poolCache.dir,"daily_"+date+"_v1.json")
 if err:=os.WriteFile(path,[]byte(strings.Repeat("x",30)),0600);err!=nil{t.Fatal(err)}
 if _,err:=r.poolCache.readDate(context.Background(),"daily",date);err==nil{
  t.Fatal("corrupt cache file was silently trusted or refetched")
 }
 capped:=marketDayBody{Version:marketDaySnapshotVersion,API:"daily",Date:date}
 for i:=0;i<marketDayMaxRows;i++{
  capped.Prices=append(capped.Prices,marketPriceRow{Symbol:fmt.Sprintf("%06d.SZ",i),Date:date,
   Open:10,High:11,Low:9,Close:10,Volume:100,Amount:1000})
 }
 if err:=validMarketBody(capped);err==nil{t.Fatal("provider cap was treated as complete data")}
 if f.count("daily")!=1 {t.Fatal("bad cache caused network retry")}
}
func TestMarketDayCacheResumeOnlyMissingDates(t *testing.T){
 const asOf="20261022"
 f:=&dayFixture{}
 r,done:=newFakeMarketDay(t,f,filepath.Join(t.TempDir(),"cache"));defer done()
 stop:=errors.New("operator interrupted after ten days")
 err:=r.PreparePool(context.Background(),asOf,[]string{"600519.SH"},func(n,total int)error{
  if n==10{return stop};return nil
 })
 if !errors.Is(err,stop){t.Fatalf("progress cancellation lost: %v",err)}
 if f.count("daily")!=10||f.count("adj_factor")!=10{t.Fatalf("nonbounded prefetch %+v",f.counts)}
 if err:=r.PreparePool(context.Background(),asOf,[]string{"600519.SH"},nil);err!=nil{t.Fatal(err)}
 if f.count("daily")!=90||f.count("adj_factor")!=90{t.Fatalf("reused dates were fetched again %+v",f.counts)}
}

func TestMarketDayCacheRejectsGrossIncompleteUniverse(t *testing.T){
 f:=&dayFixture{}
 r,done:=newFakeMarketDay(t,f,filepath.Join(t.TempDir(),"cache"));defer done()
 symbols:=make([]string,0,100)
 for i:=0;i<100;i++ {symbols=append(symbols,fmt.Sprintf("600%03d.SH",i))}
 err:=r.PreparePool(context.Background(),"20261022",symbols,nil)
 if err==nil||!strings.Contains(err.Error(),"insufficient pool coverage"){
  t.Fatalf("partial cross-section was promoted to complete pool: %v",err)
 }
 if f.count("daily")!=1||f.count("adj_factor")!=1{
  t.Fatalf("partial market data should fail on first day: %+v",f.counts)
 }
 if r.prepared!=nil{t.Fatal("rejected cache published prepared index")}
}
