package pullback

import (
 "context"
 "errors"
 "sort"
 "time"

 "Aegis/internal/capability/technical"
)

// ScanResearcher obtains source observations using the existing Technical
// Capability and applies the offline strategy. Missing/closed/stale bars never
// become selected candidates. It performs no LLM or trading operations.
// observed, when non-nil, receives each validated observation that actually
// fed an evaluation, letting callers persist the exact reviewable inputs.
func ScanResearcher(ctx context.Context, researcher technical.Researcher, asOf string, symbols []string, observed func(technical.HistoricalObservation)) ([]Result,error) {
 d,err:=time.Parse("20060102",asOf)
 if err!=nil || d.Format("20060102")!=asOf{return nil,errors.New("invalid scan date")}
 if researcher==nil{return nil,errors.New("technical researcher required")}
 seen:=make(map[string]struct{},len(symbols))
 sorted:=append([]string(nil),symbols...)
 sort.Strings(sorted)
 for _,symbol:=range sorted {
  if symbol==""{return nil,errors.New("empty symbol")}
  if _,exists:=seen[symbol];exists{return nil,errors.New("duplicate stock in scan universe")}
  seen[symbol]=struct{}{}
 }
 results:=make([]Result,0,len(sorted))
 for _,symbol:=range sorted {
  if err:=ctx.Err();err!=nil{return nil,err}
  o,ok,err:=technical.ObserveHistorical(ctx,researcher,technical.Request{Symbol:symbol,AsOf:asOf})
  if err!=nil{return nil,err}
  if !ok {
   results=append(results,Result{Symbol:symbol,AsOf:asOf,Version:StrategyVersion,Verdict:InsufficientData,Reason:"NO_VALID_HISTORICAL_OBSERVATION"})
   continue
  }
  if observed!=nil {observed(o)}
  result,err:=EvaluateObservation(o)
  if err!=nil{return nil,err}
  results=append(results,result)
 }
 return results,nil
}
