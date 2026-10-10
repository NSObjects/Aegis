package research

import (
 "Aegis/internal/portfolio"
 "Aegis/internal/portfoliodecision"
 "testing"
)

func TestEvaluateFinalRiskGate(t *testing.T){
 tests:=[]struct{name string; base portfoliodecision.Disposition; status portfolio.LimitReviewStatus; want portfoliodecision.Disposition; reason string}{
 {"breach blocks review",portfoliodecision.DispositionReviewRequired,portfolio.LimitReviewBreach,portfoliodecision.DispositionBlocked,"CURRENT_LIMIT_BREACH"},
 {"breach does not trigger a trade for monitor",portfoliodecision.DispositionNoAction,portfolio.LimitReviewBreach,portfoliodecision.DispositionNoAction,"NO_ACTION_WITH_CURRENT_LIMIT_BREACH"},
 {"missing valuation stays review",portfoliodecision.DispositionReviewRequired,portfolio.LimitReviewInsufficientData,portfoliodecision.DispositionReviewRequired,"CURRENT_LIMITS_INSUFFICIENT_DATA"},
 {"pass never approves candidate",portfoliodecision.DispositionReviewRequired,portfolio.LimitReviewPass,portfoliodecision.DispositionReviewRequired,"PORTFOLIO_DECISION"},
 {"existing block remains",portfoliodecision.DispositionBlocked,portfolio.LimitReviewPass,portfoliodecision.DispositionBlocked,"PORTFOLIO_DECISION"},
 }
 for _,tt:=range tests{
  t.Run(tt.name,func(t *testing.T){
   d:=&portfoliodecision.Decision{Disposition:tt.base}
   limits:=&portfolio.LimitReview{PolicyID:"caller-policy",Status:tt.status}
   gate,err:=evaluateFinalRiskGate(d,limits)
   if err!=nil {t.Fatal(err)}
   if gate.Disposition!=tt.want||gate.Reason!=tt.reason||gate.PolicyID!="caller-policy"||gate.CurrentLimitsStatus!=tt.status{t.Fatalf("gate = %+v",gate)}
   if d.Disposition!=tt.base{t.Fatal("must not mutate canonical decision")}
  })
 }
 without,err:=evaluateFinalRiskGate(&portfoliodecision.Decision{Disposition:portfoliodecision.DispositionReviewRequired},nil)
 if err!=nil||without.Disposition!=portfoliodecision.DispositionReviewRequired||without.PolicyID!=""{t.Fatalf("omitted limits: %+v, %v",without,err)}
 if _,err:=evaluateFinalRiskGate(nil,nil);err==nil{t.Fatal("nil decision accepted")}
 if _,err:=evaluateFinalRiskGate(&portfoliodecision.Decision{},&portfolio.LimitReview{Status:"INVENTED"});err==nil{t.Fatal("unknown limit status accepted")}
}
