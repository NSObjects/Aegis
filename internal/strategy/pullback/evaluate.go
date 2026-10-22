// Package pullback implements an offline, deterministic candidate rule.
// It neither downloads data nor invokes an LLM or trading executor.
package pullback

import (
 "errors"
 "math"
 "strings"
 "time"

 "Aegis/internal/capability/technical"
)

const StrategyVersion = "trend-pullback-v1"

type Verdict string
const (
 Selected Verdict = "SELECTED"
 Rejected Verdict = "REJECTED"
 InsufficientData Verdict = "INSUFFICIENT_DATA"
)

type Result struct {
 Symbol string `json:"symbol"`
 AsOf string `json:"as_of"`
 Version string `json:"version"`
 Verdict Verdict `json:"verdict"`
 Reason string `json:"reason"`
}

// Evaluate requires chronological, unadjusted observed bars and an exact
// as-of trading date; no inference about missing, halted or future candles.
// SMA20 > SMA60, SMA60 rising over five observed sessions, and close within
// [SMA20, SMA20*1.03] define this explicit first candidate rule.
// Raw prices may not be mixed with adjusted prices in a single evaluation.
func Evaluate(symbol, asOf string, bars []technical.HistoricalBar) (Result,error) {
 out:=Result{Symbol:symbol,AsOf:asOf,Version:StrategyVersion,Verdict:InsufficientData,Reason:"MISSING_BARS"}
 if _,err:=time.Parse("20060102",asOf);err!=nil || len(asOf)!=8{return out,errors.New("invalid as-of date")}
 if len(symbol)!=9 || (!strings.HasSuffix(symbol,".SH") && !strings.HasSuffix(symbol,".SZ")) {return out,errors.New("invalid stock symbol")}
 for i:=0;i<6;i++ {if symbol[i]<'0' || symbol[i]>'9' {return out,errors.New("invalid stock symbol")}}
 if !(strings.HasPrefix(symbol,"600")||strings.HasPrefix(symbol,"601")||strings.HasPrefix(symbol,"603")||strings.HasPrefix(symbol,"605")||strings.HasPrefix(symbol,"000")||strings.HasPrefix(symbol,"001")||strings.HasPrefix(symbol,"002")||strings.HasPrefix(symbol,"003")) {
  out.Verdict=Rejected;out.Reason="OUTSIDE_STOCK_UNIVERSE";return out,nil
 }
 previous:=""
 for _,b:=range bars {
  d,err:=time.Parse("20060102",b.TradeDate)
  if err!=nil || d.Format("20060102")!=b.TradeDate || b.TradeDate<=previous || b.TradeDate>asOf {return out,errors.New("invalid chronology or future bar")}
  previous=b.TradeDate
  for _,v:=range []float64{b.Open,b.High,b.Low,b.Close,b.Volume,b.Amount} {
   if math.IsNaN(v)||math.IsInf(v,0)||v<0{return out,errors.New("invalid OHLCV")}
  }
  if b.Open<=0||b.Close<=0||b.Low<=0||b.High<b.Low||b.High<math.Max(b.Open,b.Close)||b.Low>math.Min(b.Open,b.Close) {return out,errors.New("inconsistent OHLC")}
 }
 if len(bars)<65 || bars[len(bars)-1].TradeDate!=asOf {return out,nil}
 last:=bars[len(bars)-1]
 if last.Volume==0 {out.Reason="HALTED_OR_NO_VOLUME";return out,nil}
 sma:=func(end,n int)float64{v:=0.0;for _,b:=range bars[end-n:end]{v+=b.Close};return v/float64(n)}
 n:=len(bars);ma20:=sma(n,20);ma60:=sma(n,60);old60:=sma(n-5,60)
 out.Verdict=Rejected
 if ma20<=ma60 || ma60<=old60 {out.Reason="TREND_NOT_UP";return out,nil}
 if last.Close<ma20 || last.Close>ma20*1.03 {out.Reason="NOT_NEAR_MA20";return out,nil}
 out.Verdict=Selected;out.Reason="TREND_PULLBACK_V1";return out,nil
}
