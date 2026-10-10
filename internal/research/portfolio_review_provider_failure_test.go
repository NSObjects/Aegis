package research

import (
 "Aegis/internal/portfolio"
 "Aegis/internal/risk"
 "Aegis/internal/tradeintent"
 "Aegis/internal/research/synthesis"
 "Aegis/internal/research/debate"
 "context"
 "errors"
 "strings"
 "testing"
 "github.com/cloudwego/eino/components/model"
)

type failedRiskModel struct{ called int }
func (r *failedRiskModel) Review(_ context.Context,_ *tradeintent.Intent,_ *portfolio.Context,_ ...model.Option)(*risk.Context,error){
 r.called++
 return nil,errors.New("injected model/provider unavailable")
}

type failedDebateModel struct{ called int }
func (r *failedDebateModel) Run(_ context.Context,_ *synthesis.Artifact)(*debate.State,error){
 r.called++
 return nil,errors.New("injected model/provider unavailable")
}

func TestPortfolioReviewFailsClosedWhenRiskModelUnavailable(t *testing.T){
 w,_,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
 failing:=&failedRiskModel{}
 w.cfg.RiskAnalyst=failing
 result,err:=w.Run(context.Background(),reviewArtifactFixture(),reviewBookFixture())
 if result!=nil||err==nil||!strings.Contains(err.Error(),"risk analyst:")||!strings.Contains(err.Error(),"provider unavailable"){
  t.Fatalf("must not produce portfolio decision after risk provider failure: result=%+v err=%v",result,err)
 }
 if failing.called!=1{t.Fatalf("risk calls=%d",failing.called)}
}
