package pullback

import (
 "errors"
 "sort"

 "Aegis/internal/capability/technical"
)

// Scan evaluates a caller-supplied point-in-time universe without querying
// providers, invoking an LLM, or initiating any trading action. Every member
// must have a unique symbol and match the same requested as-of cutoff.
func Scan(asOf string, observations []technical.HistoricalObservation) ([]Result,error) {
 if asOf=="" {return nil,errors.New("scan as-of required")}
 seen:=make(map[string]struct{},len(observations))
 results:=make([]Result,0,len(observations))
 for _,o:=range observations {
  if o.Symbol=="" || o.RequestedAsOf!=asOf {return nil,errors.New("scan identity/as-of mismatch")}
  if _,exists:=seen[o.Symbol];exists{return nil,errors.New("duplicate stock in scan universe")}
  seen[o.Symbol]=struct{}{}
  result,err:=EvaluateObservation(o)
  if err!=nil{return nil,err}
  results=append(results,result)
 }
 sort.Slice(results,func(i,j int)bool{return results[i].Symbol<results[j].Symbol})
 return results,nil
}
