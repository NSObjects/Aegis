package technical

import (
 "context"
 "errors"
)

// HistoricalObservation is a copied, validated observation from a real technical
// tool invocation. It is not an LLM-generated response or a trading signal.
type HistoricalObservation struct {
 Symbol string
 RequestedAsOf string
 EffectiveAsOf string
 PriceBasis string
 Bars []HistoricalBar
}

type historicalObserverKey struct{}

func WithHistoricalObserver(ctx context.Context, sink func(HistoricalObservation)) context.Context {
 if ctx==nil || sink==nil {return ctx}
 return context.WithValue(ctx,historicalObserverKey{},sink)
}

func reportHistoricalObservation(ctx context.Context, req Request, result map[string]any) error {
 if ctx==nil{return nil}
 sink,_:=ctx.Value(historicalObserverKey{}).(func(HistoricalObservation))
 if sink==nil{return nil}
 if result["status"]!="NORMAL" {return nil}
 raw,ok:=result["historical_ohlcv"].(map[string]any)
 if !ok{return nil}
 if raw["version"]!=1 || raw["price_basis"]!="RAW" {return errors.New("unsupported historical series format")}
 bars,ok:=raw["bars"].([]HistoricalBar)
 if !ok || len(bars)==0 {return nil}
 symbol,_:=result["symbol"].(string)
 asof,_:=result["requested_as_of"].(string)
 effective,_:=result["effective_as_of"].(string)
 if symbol!=req.Symbol || asof!=req.AsOf || effective=="" || bars[len(bars)-1].TradeDate!=effective || effective>asof {
  return errors.New("historical observation identity mismatch")
 }
 // Never share the mutable tool result backing array with durable observers.
 cloned:=append([]HistoricalBar(nil),bars...)
 sink(HistoricalObservation{Symbol:symbol,RequestedAsOf:asof,EffectiveAsOf:effective,PriceBasis:"RAW",Bars:cloned})
 return nil
}
