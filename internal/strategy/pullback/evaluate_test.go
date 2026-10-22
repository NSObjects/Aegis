package pullback

import (
 "testing"
 "time"

 "Aegis/internal/capability/technical"
)
func risingBars() []technical.HistoricalBar {
 bars:=make([]technical.HistoricalBar,65)
 for i:=range bars {
  price:=100+float64(i)*0.5
  bars[i]=technical.HistoricalBar{TradeDate:time.Date(2026,1,1,0,0,0,0,time.UTC).AddDate(0,0,i).Format("20060102"),Open:price,High:price+1,Low:price-1,Close:price,Volume:1000,Amount:100000}
 }
 return bars
}
func TestEvaluateDeterminismAndSafety(t *testing.T){
 bars:=risingBars()
 // Use actual chronological calendar dates rather than manufactured days.
 // 65 trading-session samples; the algorithm requires only ordered observations.
 asOf:=bars[len(bars)-1].TradeDate
 got,err:=Evaluate("600519.SH",asOf,bars)
 if err!=nil {t.Fatal(err)}
 again,err:=Evaluate("600519.SH",asOf,bars)
 if err!=nil||got!=again {t.Fatalf("not deterministic: %+v %+v %v",got,again,err)}
 if got.Version!=StrategyVersion {t.Fatal("unversioned result")}
 stopped:=append([]technical.HistoricalBar(nil),bars...)
 stopped[len(stopped)-1].Volume=0
 result,err:=Evaluate("600519.SH",asOf,stopped)
 if err!=nil||result.Verdict!=InsufficientData {t.Fatalf("zero volume accepted: %+v %v",result,err)}
 missing,err:=Evaluate("600519.SH",asOf,bars[:64])
 if err!=nil||missing.Verdict!=InsufficientData {t.Fatalf("missing cutoff selected: %+v %v",missing,err)}
 if _,err:=Evaluate("600519.SH",asOf,bars[:64]);err!=nil {t.Fatal(err)}
 future:=append([]technical.HistoricalBar(nil),bars...)
 future[len(future)-1].TradeDate="20280101"
 if _,err:=Evaluate("600519.SH",asOf,future);err==nil {t.Fatal("future bar accepted")}
 invalid:=append([]technical.HistoricalBar(nil),bars...)
 invalid[5].TradeDate=invalid[4].TradeDate
 if _,err:=Evaluate("600519.SH",asOf,invalid);err==nil {t.Fatal("duplicate date accepted")}
 excluded,err:=Evaluate("300001.SZ",asOf,bars)
 if err!=nil||excluded.Reason!="OUTSIDE_STOCK_UNIVERSE" {t.Fatalf("universe breach: %+v %v",excluded,err)}
}

func TestEvaluateSelectsObservedPullback(t *testing.T) {
 bars:=risingBars()
 last:=len(bars)-1
 bars[last].Open=129
 bars[last].High=130
 bars[last].Low=128
 bars[last].Close=129
 asOf:=bars[last].TradeDate
 result,err:=Evaluate("600519.SH",asOf,bars)
 if err!=nil||result.Verdict!=Selected||result.Reason!="TREND_PULLBACK_V1" {
  t.Fatalf("valid upward trend pullback not selected: %+v %v",result,err)
 }
 if _,err:=Evaluate("600abc.SH",asOf,bars);err==nil {t.Fatal("non-numeric symbol accepted")}
}
