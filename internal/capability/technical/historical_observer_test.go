package technical

import (
 "context"
 "testing"
)

func TestHistoricalObservationCopiesRealSeries(t *testing.T) {
 original:=[]HistoricalBar{{TradeDate:"20261009",Open:10,High:12,Low:9,Close:11,Volume:100}}
 req:=Request{Symbol:"600519.SH",AsOf:"20261009"}
 result:=map[string]any{
  "status":"NORMAL","symbol":req.Symbol,"requested_as_of":req.AsOf,"effective_as_of":req.AsOf,
  "historical_ohlcv":map[string]any{"version":1,"price_basis":"RAW","bars":original},
 }
 var seen HistoricalObservation
 ctx:=WithHistoricalObserver(context.Background(),func(x HistoricalObservation){seen=x})
 if err:=reportHistoricalObservation(ctx,req,result);err!=nil{t.Fatal(err)}
 if len(seen.Bars)!=1 || seen.Symbol!=req.Symbol || seen.PriceBasis!="RAW"{t.Fatalf("unexpected observed series %#v",seen)}
 original[0].Close=999
 if seen.Bars[0].Close!=11{t.Fatal("observer shares mutable source array")}
}

func TestHistoricalObservationRejectsCrossRunOrFutureData(t *testing.T) {
 req:=Request{Symbol:"600519.SH",AsOf:"20261009"}
 called:=false
 ctx:=WithHistoricalObserver(context.Background(),func(HistoricalObservation){called=true})
 for _,tc:=range []struct{name,symbol,date string}{
  {"wrong-symbol","000001.SZ","20261009"},
  {"future-date","600519.SH","20261012"},
 } {
  t.Run(tc.name,func(t *testing.T){
   result:=map[string]any{"status":"NORMAL","symbol":tc.symbol,"requested_as_of":req.AsOf,"effective_as_of":tc.date,
    "historical_ohlcv":map[string]any{"version":1,"price_basis":"RAW","bars":[]HistoricalBar{{TradeDate:tc.date,Open:10,High:12,Low:9,Close:11}}}}
   if err:=reportHistoricalObservation(ctx,req,result);err==nil{t.Fatal("unsafe series accepted")}
  })
 }
 if called {t.Fatal("invalid observation reached sink")}
}

func TestHistoricalObservationUnavailableIsNotInvented(t *testing.T) {
 called:=false
 ctx:=WithHistoricalObserver(context.Background(),func(HistoricalObservation){called=true})
 if err:=reportHistoricalObservation(ctx,Request{Symbol:"600519.SH",AsOf:"20261009"},map[string]any{"status":"NO_DATA"});err!=nil{t.Fatal(err)}
 if called{t.Fatal("fabricated historical bars")}
}
