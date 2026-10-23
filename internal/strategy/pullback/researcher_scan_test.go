package pullback

import (
 "context"
 "errors"
 "testing"

 "Aegis/internal/capability/technical"
)
type scanResearcherStub struct {calls []string; fail bool}
func (s *scanResearcherStub) Research(_ context.Context,req technical.Request)(map[string]any,error){
 s.calls=append(s.calls,req.Symbol)
 if s.fail{return nil,errors.New("provider failure")}
 if req.Symbol=="000001.SZ"{return map[string]any{"status":"NO_DATA"},nil}
 bars:=risingBars()
 factors:=make([]technical.PITAdjustmentFactor,len(bars))
 for i,b:=range bars{factors[i]=technical.PITAdjustmentFactor{TradeDate:b.TradeDate,Factor:1}}
 return map[string]any{
  "status":"NORMAL","symbol":req.Symbol,"requested_as_of":req.AsOf,"effective_as_of":bars[len(bars)-1].TradeDate,
  "historical_ohlcv":map[string]any{"version":1,"price_basis":"RAW","bars":bars,"pit_adjustment_factors":factors},
 },nil
}
func TestScanResearcherUsesSourceObservations(t *testing.T){
 bars:=risingBars();date:=bars[len(bars)-1].TradeDate
 stub:=&scanResearcherStub{}
 results,err:=ScanResearcher(context.Background(),stub,date,[]string{"600519.SH","000001.SZ"})
 if err!=nil||len(results)!=2||results[0].Symbol!="000001.SZ"||results[0].Verdict!=InsufficientData||results[1].Symbol!="600519.SH"{
  t.Fatalf("unexpected provider scan %+v %v",results,err)
 }
 if len(stub.calls)!=2||stub.calls[0]!="000001.SZ"{t.Fatalf("scan order not deterministic %+v",stub.calls)}
 if _,err:=ScanResearcher(context.Background(),stub,date,[]string{"600519.SH","600519.SH"});err==nil{t.Fatal("duplicate accepted")}
 failing:=&scanResearcherStub{fail:true}
 if results,err:=ScanResearcher(context.Background(),failing,date,[]string{"600519.SH"});err==nil||results!=nil{t.Fatalf("provider errors swallowed %+v %v",results,err)}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=ScanResearcher(ctx,stub,date,[]string{"600519.SH"});err==nil{t.Fatal("cancelled scan accepted")}
}
