package pullback

import (
 "testing"

 "Aegis/internal/capability/technical"
)

func TestScanDeterminismAndFailClosed(t *testing.T) {
 bars:=risingBars()
 n:=len(bars)-1
 bars[n].Open=129;bars[n].High=130;bars[n].Low=128;bars[n].Close=129
 factors:=make([]technical.PITAdjustmentFactor,len(bars))
 for i,b:=range bars {factors[i]=technical.PITAdjustmentFactor{TradeDate:b.TradeDate,Factor:1}}
 date:=bars[n].TradeDate
 build:=func(symbol string)technical.HistoricalObservation{return technical.HistoricalObservation{
  Symbol:symbol,RequestedAsOf:date,EffectiveAsOf:date,PriceBasis:"RAW",Bars:bars,PITFactors:factors,
 }}
 first,err:=Scan(date,[]technical.HistoricalObservation{build("600519.SH"),build("000001.SZ")})
 if err!=nil||len(first)!=2||first[0].Symbol!="000001.SZ"||first[0].Verdict!=Selected{t.Fatalf("unexpected scan result %+v %v",first,err)}
 second,err:=Scan(date,[]technical.HistoricalObservation{build("000001.SZ"),build("600519.SH")})
 if err!=nil||first[0]!=second[0]||first[1]!=second[1]{t.Fatalf("scan depends on input order %+v %+v %v",first,second,err)}
 if _,err:=Scan(date,[]technical.HistoricalObservation{build("600519.SH"),build("600519.SH")});err==nil{t.Fatal("duplicate accepted")}
 stale:=build("600519.SH");stale.RequestedAsOf="20261231"
 if _,err:=Scan(date,[]technical.HistoricalObservation{stale});err==nil{t.Fatal("mixed as-of accepted")}
 missing:=build("600519.SH");missing.PITFactors=nil
 rejected,err:=Scan(date,[]technical.HistoricalObservation{missing})
 if err!=nil||len(rejected)!=1||rejected[0].Verdict!=InsufficientData{t.Fatalf("missing factors selected %+v %v",rejected,err)}
}
