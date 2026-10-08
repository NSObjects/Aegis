package research

import (
	portfolioagent "Aegis/internal/agent/portfolio"
	"Aegis/internal/portfolio"
	"Aegis/internal/portfoliodecision"
	"Aegis/internal/research/debate"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
)

type reviewDebateStub struct {
	state *debate.State
	calls int
}

func (s *reviewDebateStub) Run(context.Context, *synthesis.Artifact) (*debate.State, error) {
	s.calls++
	return s.state, nil
}

type reviewManagerStub struct {
	plan *manager.ResearchPlan
	calls int
}

func (s *reviewManagerStub) BuildPlan(
	_ context.Context, _ *synthesis.Artifact, _ *debate.State, _ ...model.Option,
) (*manager.ResearchPlan, error) {
	s.calls++
	return s.plan, nil
}

type reviewTraderStub struct {
	intent *tradeintent.Intent
	calls int
}

func (s *reviewTraderStub) AnalyzeResearchPlanIntent(
	_ context.Context, _ *synthesis.Artifact, _ *manager.ResearchPlan, _ ...adk.AgentRunOption,
) (*tradeintent.Intent, error) {
	s.calls++
	return s.intent, nil
}

type reviewRiskStub struct {
	review *risk.Context
	calls int
	lastBook *portfolio.Context
}

func (s *reviewRiskStub) Review(
	_ context.Context, _ *tradeintent.Intent, book *portfolio.Context, _ ...model.Option,
) (*risk.Context, error) {
	s.calls++
	s.lastBook = book
	return s.review, nil
}

func reviewArtifactFixture() *synthesis.Artifact {
	return &synthesis.Artifact{
		Symbol: "600519.SH",
		Market: validMarketContext(),
		Technical: validTechnicalContext(),
		Event: validEventContext(),
		Synthesis: synthesis.Synthesis{
			Summary: "grounded synthesis",
			Confidence: 0.72,
		},
	}
}

func reviewDebateFixture() *debate.State {
	return &debate.State{
		Symbol: "600519.SH",
		Turns: []debate.Turn{
			{
				Round: 1, Side: debate.SideBull, Thesis: "constructive",
				Arguments: []debate.Argument{
					{ID: "bull-1", Claim: "technical setup", References: []synthesis.Reference{{Domain:synthesis.DomainTechnical}}},
				},
				Confidence: 0.7,
			},
			{
				Round: 1, Side: debate.SideBear, Thesis: "cautious",
				Arguments: []debate.Argument{
					{ID: "bear-1", Claim: "market risk", References: []synthesis.Reference{{Domain:synthesis.DomainMarket}}},
				},
				Confidence: 0.6,
			},
		},
	}
}

func reviewPlanFixture() *manager.ResearchPlan {
	return &manager.ResearchPlan{
		Symbol: "600519.SH",
		Conclusion: manager.ConclusionBalanced,
		Summary: "both sides have evidence",
		AcceptedArgumentIDs: []string{"bull-1"},
		UnresolvedArgumentIDs: []string{"bear-1"},
		Confidence: 0.65,
	}
}

func reviewIntentFixture() *tradeintent.Intent {
	return &tradeintent.Intent{
		Symbol: "600519.SH",
		Thesis: "candidate requires risk review",
		Evidence: []string{"supported by research"},
		Risk: []string{"market weakness"},
		Status: tradeintent.StatusCandidate,
		Confidence: 0.65,
	}
}

func reviewBookFixture() *portfolio.Context {
	return &portfolio.Context{
		State: portfolio.StatePositions,
		AsOf: "2026-10-08T09:00:00+08:00",
		Positions: []portfolio.Position{
			{Symbol:"600519.SH", Quantity:100},
		},
	}
}

func reviewContextFixture(assessment risk.Assessment) *risk.Context {
	return &risk.Context{
		Symbol:"600519.SH",
		Assessment: assessment,
		Summary:"risk reviewed with portfolio data",
		Findings: []risk.Item{
			{Claim:"observed position requires review", References: []risk.Reference{{Domain:risk.ReferencePortfolio, Symbol:"600519.SH"}}},
		},
		Confidence: 0.73,
	}
}

func reviewWorkflowFixture(t *testing.T, review *risk.Context) (*PortfolioReview, *reviewDebateStub, *reviewManagerStub, *reviewTraderStub, *reviewRiskStub) {
	t.Helper()
	d := &reviewDebateStub{state:reviewDebateFixture()}
	m := &reviewManagerStub{plan:reviewPlanFixture()}
	tr := &reviewTraderStub{intent:reviewIntentFixture()}
	r := &reviewRiskStub{review:review}
	w,err:=NewPortfolioReview(PortfolioReviewConfig{
		Debate:d,
		ResearchManager:m,
		TradingAgent:tr,
		RiskAnalyst:r,
		PortfolioManager:&portfolioagent.Manager{},
	})
	if err!=nil {t.Fatal(err)}
	return w,d,m,tr,r
}

func TestPortfolioReviewCompleteBoundary(t *testing.T) {
	w,d,m,tr,r:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
	book:=reviewBookFixture()
	result,err:=w.Run(context.Background(),reviewArtifactFixture(),book)
	if err!=nil {t.Fatalf("Run() error = %v",err)}
	if result.PortfolioDecision.Disposition!=portfoliodecision.DispositionBlocked {
		t.Fatalf("disposition = %q",result.PortfolioDecision.Disposition)
	}
	if len(result.Debate.Turns)!=2 || result.ResearchPlan.Conclusion!=manager.ConclusionBalanced {
		t.Fatalf("upstream outputs lost: %+v",result)
	}
	if result.TradeIntent.Status!=tradeintent.StatusCandidate || result.RiskContext.Assessment!=risk.AssessmentHighRisk {
		t.Fatalf("typed stage output mismatch: %+v",result)
	}
	if d.calls!=1 || m.calls!=1 || tr.calls!=1 || r.calls!=1 || r.lastBook!=book {
		t.Fatalf("calls debate=%d manager=%d trader=%d risk=%d, book preserved=%v",
			d.calls,m.calls,tr.calls,r.calls,r.lastBook==book)
	}
}

func TestPortfolioReviewMissingBookFailsBeforeDebate(t *testing.T) {
	w,d,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
	_,err:=w.Run(context.Background(),reviewArtifactFixture(),nil)
	if err==nil || !strings.Contains(err.Error(),"invalid portfolio context") {t.Fatalf("error = %v",err)}
	if d.calls!=0 {t.Fatalf("debate calls = %d, want 0",d.calls)}
}

func TestPortfolioReviewNotProvidedIsExplicit(t *testing.T) {
	w,_,_,_,r:=reviewWorkflowFixture(t,&risk.Context{
		Symbol:"600519.SH",
		Assessment:risk.AssessmentInsufficientContext,
		Summary:"portfolio not observed",
		Confidence:1,
	})
	book:=&portfolio.Context{State:portfolio.StateNotProvided}
	result,err:=w.Run(context.Background(),reviewArtifactFixture(),book)
	if err!=nil {t.Fatalf("Run() error = %v",err)}
	if r.lastBook!=book {t.Fatal("explicit NOT_PROVIDED state was not passed through")}
	if result.PortfolioDecision.Disposition!=portfoliodecision.DispositionReviewRequired ||
		result.PortfolioDecision.ReasonCode!=portfoliodecision.ReasonRiskUncertain {
		t.Fatalf("invalid disposition for missing portfolio: %+v",result.PortfolioDecision)
	}
}

func TestPortfolioReviewRejectsInventedRisk(t *testing.T) {
	bad:=reviewContextFixture(risk.AssessmentHighRisk)
	bad.Findings[0].References[0].Symbol="000001.SZ"
	w,_,_,_,_:=reviewWorkflowFixture(t,bad)
	_,err:=w.Run(context.Background(),reviewArtifactFixture(),reviewBookFixture())
	if err==nil || !strings.Contains(err.Error(),"invalid risk context") {t.Fatalf("error = %v",err)}
}

func TestPortfolioReviewRejectsMismatchedIntent(t *testing.T) {
	w,_,_,tr,r:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
	tr.intent.Symbol="000001.SZ"
	_,err:=w.Run(context.Background(),reviewArtifactFixture(),reviewBookFixture())
	if err==nil || !strings.Contains(err.Error(),"does not match artifact symbol") {t.Fatalf("error = %v",err)}
	if r.calls!=0 {t.Fatal("risk reviewer must not run after invalid trading output")}
}

func TestPortfolioReviewRequiresDebate(t *testing.T) {
	w,d,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
	d.state=&debate.State{Symbol:"600519.SH"}
	_,err:=w.Run(context.Background(),reviewArtifactFixture(),reviewBookFixture())
	if err==nil || !strings.Contains(err.Error(),"no turns") {t.Fatalf("error = %v",err)}
}

func TestPortfolioReviewCancelled(t *testing.T) {
	w,d,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
	ctx,cancel:=context.WithCancel(context.Background())
	cancel()
	_,err:=w.Run(ctx,reviewArtifactFixture(),reviewBookFixture())
	if !errors.Is(err,context.Canceled) {t.Fatalf("error = %v",err)}
	if d.calls!=0 {t.Fatal("debate ran after context cancelled")}
}

func TestPortfolioReviewRequiresAllRoles(t *testing.T) {
	_,err:=NewPortfolioReview(PortfolioReviewConfig{})
	if err==nil || !strings.Contains(err.Error(),"debate") {t.Fatalf("error = %v",err)}
}

func TestPortfolioReviewReturnsObservedCurrentExposure(t *testing.T) {
	w,_,_,_,riskAnalyst:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentCaution))
	book:=reviewBookFixture()
	book.Valuation=&portfolio.Valuation{
		Scope:portfolio.ValuationScopeCashAndPositions, Currency:portfolio.CurrencyCNY,
		TotalValueFen:100000, CashFen:40000,
		PositionValues:[]portfolio.PositionValue{{Symbol:"600519.SH",MarketValueFen:60000}},
	}
	result,err:=w.Run(context.Background(),reviewArtifactFixture(),book)
	if err!=nil {t.Fatal(err)}
	if result.ObservedExposure==nil ||
		len(result.ObservedExposure.Positions)!=1 ||
		result.ObservedExposure.Positions[0].WeightBps!=6000 ||
		result.ObservedExposure.CashWeightBps==nil ||
		*result.ObservedExposure.CashWeightBps!=4000 {
		t.Fatalf("missing audited exposure in review artifact: %+v",result.ObservedExposure)
	}
	if riskAnalyst.lastBook!=book {
		t.Fatal("risk analyst must receive original observed valuation")
	}
	if result.PortfolioDecision.Disposition!=portfoliodecision.DispositionReviewRequired {
		t.Fatal("observed valuation without risk budget or trade size must not approve execution")
	}
}

func TestPortfolioReviewNeverInventsExposure(t *testing.T) {
	w,_,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentCaution))
	result,err:=w.Run(context.Background(),reviewArtifactFixture(),reviewBookFixture())
	if err!=nil {t.Fatal(err)}
	if result.ObservedExposure!=nil {
		t.Fatalf("missing valuation must not be represented as zero or known: %+v",result.ObservedExposure)
	}
}

func TestPortfolioReviewRejectsUnreconciledValuationBeforeDebate(t *testing.T) {
	w,debateAgent,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentCaution))
	book:=reviewBookFixture()
	book.Valuation=&portfolio.Valuation{
		Scope:portfolio.ValuationScopeCashAndPositions, Currency:portfolio.CurrencyCNY,
		TotalValueFen:100000, CashFen:50000,
		PositionValues:[]portfolio.PositionValue{{Symbol:"600519.SH",MarketValueFen:10000}},
	}
	result,err:=w.Run(context.Background(),reviewArtifactFixture(),book)
	if err==nil || result!=nil || !strings.Contains(err.Error(),"does not reconcile") || debateAgent.calls!=0 {
		t.Fatalf("must fail before model-driven debate: result=%+v err=%v debate=%d",result,err,debateAgent.calls)
	}
}
