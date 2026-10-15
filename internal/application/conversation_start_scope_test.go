package application

import (
 "context"
 "testing"
 "time"
)

func TestEinoStartResearchIsBoundToAuthorizedTicker(t *testing.T) {
 stub:=&conversationRunsStub{records:map[string]RunRecord{}}
 tools:=&ConversationResearchTools{runs:stub}
 cutoff:=time.Date(2026,10,15,9,0,0,0,time.UTC)
 if _,err:=tools.EinoStartResearchTool(ConversationStartPolicy{Cutoff:cutoff,Lookback:time.Hour,AllowedSymbol:"INVALID"});err==nil {t.Fatal("accepted invalid authorization scope")}
 start,err:=tools.EinoStartResearchTool(ConversationStartPolicy{Cutoff:cutoff,Lookback:time.Hour,AllowedSymbol:"600519.SH"})
 if err!=nil {t.Fatal(err)}
 if _,err:=start.InvokableRun(context.Background(),`{"symbol":"000001.SZ"}`);err==nil {t.Fatal("model submitted an unapproved different ticker")}
 if stub.submitted.Input.Symbol!="" {t.Fatalf("unauthorized call submitted: %+v",stub.submitted)}
 out,err:=start.InvokableRun(context.Background(),`{"symbol":"600519.SH"}`)
 if err!=nil||!containsJSONField(out,"run_id","new-run"){t.Fatalf("authorized start: %q %v",out,err)}
}
