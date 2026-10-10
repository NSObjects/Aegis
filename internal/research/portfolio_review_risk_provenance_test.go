package research

import (
 "Aegis/internal/portfolio"
 "Aegis/internal/portfoliodecision"
 "Aegis/internal/risk"
 "Aegis/internal/tradeintent"
 "context"
 "strings"
 "testing"
)

type modifyingRiskContextPortfolioManager struct{}
func (*modifyingRiskContextPortfolioManager) Review(_ context.Context,_ *tradeintent.Intent,_ *portfolio.Context,r *risk.Context)(*portfoliodecision.Decision,error){
 r.Summary="portfolio manager silently rewrote the upstream risk explanation"
 return nil,nil
}

func TestPortfolioReviewRejectsRiskContextMutationByPortfolioManager(t *testing.T){
 w,_,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
 w.cfg.PortfolioManager=&modifyingRiskContextPortfolioManager{}
 result,err:=w.Run(context.Background(),reviewArtifactFixture(),reviewBookFixture())
 if result!=nil||err==nil||!strings.Contains(err.Error(),"portfolio manager mutated validated risk context"){
  t.Fatalf("expected fail-closed risk provenance violation: result=%+v err=%v",result,err)
 }
}
