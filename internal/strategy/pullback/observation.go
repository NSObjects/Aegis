package pullback

import "Aegis/internal/capability/technical"

// EvaluateObservation consumes the validated historical observer payload.
// The requested cutoff must equal the observation's effective close; stale
// fallback dates are not silently treated as fresh selection candidates.
func EvaluateObservation(o technical.HistoricalObservation) (Result,error) {
 if o.PriceBasis!="RAW" {
  return Result{Symbol:o.Symbol,AsOf:o.RequestedAsOf,Version:StrategyVersion,Verdict:InsufficientData,Reason:"UNSUPPORTED_PRICE_BASIS"},nil
 }
 if o.RequestedAsOf!=o.EffectiveAsOf || o.EffectiveAsOf=="" {
  return Result{Symbol:o.Symbol,AsOf:o.RequestedAsOf,Version:StrategyVersion,Verdict:InsufficientData,Reason:"STALE_OR_MISSING_CLOSE"},nil
 }
 factors:=make([]AdjustmentFactor,len(o.PITFactors))
 for i,f:=range o.PITFactors {factors[i]=AdjustmentFactor{TradeDate:f.TradeDate,Factor:f.Factor}}
 return Evaluate(o.Symbol,o.EffectiveAsOf,o.Bars,factors)
}
