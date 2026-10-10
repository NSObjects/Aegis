package research

import (
 "Aegis/internal/portfolio"
 "Aegis/internal/portfoliodecision"
 "Aegis/internal/risk"
 "Aegis/internal/tradeintent"
 "context"
 "strings"
 "testing"
 "github.com/cloudwego/eino/components/model"
)

type mutatingRiskReviewer struct{ mutate func(*tradeintent.Intent,*portfolio.Context) }
func (s *mutatingRiskReviewer) Review(_ context.Context,intent *tradeintent.Intent,book *portfolio.Context,_ ...model.Option)(*risk.Context,error){
 s.mutate(intent,book)
 return reviewContextFixture(risk.AssessmentHighRisk),nil
}

type mutatingPortfolioReviewer struct{ mutate func(*tradeintent.Intent,*portfolio.Context) }
func (s *mutatingPortfolioReviewer) Review(_ context.Context,intent *tradeintent.Intent,book *portfolio.Context,_ *risk.Context)(*portfoliodecision.Decision,error){
 s.mutate(intent,book)
 return nil,nil
}

func TestPortfolioReviewRejectsDownstreamInputMutation(t *testing.T){
 for _,tc:=range []struct{
  name string
  stage string
  mutate func(*tradeintent.Intent,*portfolio.Context)
 }{
  {"risk changes intent","risk",func(i *tradeintent.Intent,_ *portfolio.Context){i.Symbol="000001.SZ"}},
  {"risk changes account","risk",func(_ *tradeintent.Intent,b *portfolio.Context){b.State=portfolio.StateNotProvided}},
  {"portfolio changes intent","portfolio",func(i *tradeintent.Intent,_ *portfolio.Context){i.Symbol="000001.SZ"}},
  {"portfolio changes account","portfolio",func(_ *tradeintent.Intent,b *portfolio.Context){b.State=portfolio.StateNotProvided}},
 }{
  t.Run(tc.name,func(t *testing.T){
   w,_,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
   if tc.stage=="risk"{w.cfg.RiskAnalyst=&mutatingRiskReviewer{mutate:tc.mutate}
   }else{w.cfg.PortfolioManager=&mutatingPortfolioReviewer{mutate:tc.mutate}}
   result,err:=w.Run(context.Background(),reviewArtifactFixture(),reviewBookFixture())
   if result!=nil||err==nil||!strings.Contains(err.Error(),"mutated validated trade intent or portfolio context"){
    t.Fatalf("expected fail-closed mutation error: result=%+v err=%v",result,err)
   }
  })
 }
}
