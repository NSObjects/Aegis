package technical

import (
 "errors"
 "fmt"
 "math"
)

// HistoricalBar is one observed, raw daily OHLCV bar, never a synthetic candle.
// Adjusted indicators must not be plotted against these raw prices.
type HistoricalBar struct {
 TradeDate string `json:"trade_date"`
 Open float64 `json:"open"`
 High float64 `json:"high"`
 Low float64 `json:"low"`
 Close float64 `json:"close"`
 Volume float64 `json:"volume"`
 Amount float64 `json:"amount"`
}

// observedHistoricalBars accepts chronological source rows only. It never
// interpolates missing trading dates or deduplicates conflicting source rows.
func observedHistoricalBars(bars []bar) ([]HistoricalBar,error) {
 result:=make([]HistoricalBar,0,len(bars))
 previous:=""
 for _,b:=range bars {
  if len(b.Date)!=8 || b.Date<=previous {return nil,fmt.Errorf("non-monotonic or invalid source trading date %q",b.Date)}
  for _,v:=range []float64{b.Open,b.High,b.Low,b.Close,b.Vol,b.Amount} {
   if math.IsNaN(v)||math.IsInf(v,0)||v<0{return nil,errors.New("invalid observed OHLCV value")}
  }
  if b.Open<=0||b.High<=0||b.Low<=0||b.Close<=0||b.High<math.Max(b.Open,b.Close)||b.Low>math.Min(b.Open,b.Close)||b.High<b.Low {
   return nil,errors.New("inconsistent observed OHLC")
  }
  result=append(result,HistoricalBar{TradeDate:b.Date,Open:b.Open,High:b.High,Low:b.Low,Close:b.Close,Volume:b.Vol,Amount:b.Amount})
  previous=b.Date
 }
 return result,nil
}
