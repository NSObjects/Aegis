package application

import (
 "errors"
 "fmt"
 "math"
 "time"

 technicalcap "Aegis/internal/capability/technical"
)

const maxHistoricalBars = 512

func validateHistoricalObservation(record RunRecord, obs *technicalcap.HistoricalObservation) error {
 if obs == nil { return nil }
 if record.State == RunPending { return errors.New("pending run cannot claim observed OHLCV") }
 if obs.Symbol != record.Symbol || obs.PriceBasis != "RAW" { return errors.New("observed OHLCV identity or price basis mismatch") }
 cutoff:=record.RequestedAsOf.In(time.FixedZone("A-share",8*60*60)).Format("20060102")
 if obs.RequestedAsOf!=cutoff || len(obs.Bars)==0 || len(obs.Bars)>maxHistoricalBars { return errors.New("invalid observed OHLCV cutoff or count") }
 last:=""
 for _,b:=range obs.Bars {
  day,err:=time.Parse("20060102",b.TradeDate)
  if err!=nil || day.Format("20060102")!=b.TradeDate || b.TradeDate<=last || b.TradeDate>cutoff { return fmt.Errorf("invalid observed trading date") }
  for _,v:=range []float64{b.Open,b.High,b.Low,b.Close,b.Volume,b.Amount} {
   if math.IsNaN(v)||math.IsInf(v,0)||v<0 {return errors.New("invalid historical bar value")}
  }
  if b.Open<=0 || b.Close<=0 || b.High<math.Max(b.Open,b.Close) || b.Low>math.Min(b.Open,b.Close) || b.Low<=0 || b.High<b.Low {return errors.New("invalid historical OHLC")}
  last=b.TradeDate
 }
 if last!=obs.EffectiveAsOf{return errors.New("observed OHLCV effective date mismatch")}
 return nil
}
