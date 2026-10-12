package portfoliodecision

import (
 "testing"
 "unicode"
 "Aegis/internal/risk"
 "Aegis/internal/tradeintent"
)

func TestCanonicalPolicySummariesAreChinese(t *testing.T) {
 cases:=[]struct{
  name string
  status tradeintent.Status
  assessment risk.Assessment
  observed bool
 }{
  {"monitor",tradeintent.StatusMonitor,risk.AssessmentAcceptable,false},
  {"rejected",tradeintent.StatusRejected,risk.AssessmentAcceptable,false},
  {"high risk",tradeintent.StatusCandidate,risk.AssessmentHighRisk,false},
  {"context unknown",tradeintent.StatusCandidate,risk.AssessmentInsufficientContext,false},
  {"caution",tradeintent.StatusCandidate,risk.AssessmentCaution,false},
  {"monetary exposure observed",tradeintent.StatusCandidate,risk.AssessmentAcceptable,true},
  {"monetary exposure unknown",tradeintent.StatusCandidate,risk.AssessmentAcceptable,false},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  result,err:=classify(tc.status,tc.assessment,tc.observed)
  if err!=nil{t.Fatal(err)}
  hasHan:=false
  for _,r:=range result.summary{if unicode.Is(unicode.Han,r){hasHan=true;break}}
  if !hasHan{t.Fatalf("deterministic report summary is not Chinese: %q",result.summary)}
 })}
}
