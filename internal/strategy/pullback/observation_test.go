package pullback

import (
 "testing"
 "Aegis/internal/capability/technical"
)

func TestEvaluateObservationRequiresFreshPITData(t *testing.T) {
 bars:=risingBars()
 n:=len(bars)-1
 bars[n].Open=129;bars[n].High=130;bars[n].Low=128;bars[n].Close=129
 factors:=make([]technical.PITAdjustmentFactor,len(bars))
 for i,b:=range bars {factors[i]=technical.PITAdjustmentFactor{TradeDate:b.TradeDate,Factor:1}}
 o:=technical.HistoricalObservation{Symbol:"600519.SH",RequestedAsOf:bars[n].TradeDate,EffectiveAsOf:bars[n].TradeDate,PriceBasis:"RAW",Bars:bars,PITFactors:factors}
 r,err:=EvaluateObservation(o)
 if err!=nil||r.Verdict!=Selected {t.Fatalf("observed candidate: %+v %v",r,err)}
 o.PITFactors=nil
 r,err=EvaluateObservation(o)
 if err!=nil||r.Verdict!=InsufficientData {t.Fatalf("missing factors accepted: %+v %v",r,err)}
 o.PITFactors=factors;o.RequestedAsOf="20261231"
 r,err=EvaluateObservation(o)
 if err!=nil||r.Reason!="STALE_OR_MISSING_CLOSE" {t.Fatalf("stale close accepted: %+v %v",r,err)}
 o.RequestedAsOf=o.EffectiveAsOf;o.PriceBasis="PIT_QFQ"
 r,err=EvaluateObservation(o)
 if err!=nil||r.Reason!="UNSUPPORTED_PRICE_BASIS" {t.Fatalf("mixed price basis: %+v %v",r,err)}
}
