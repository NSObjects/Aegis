package research

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/portfoliodecision"
	"Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"context"
	"strings"
	"testing"
	"time"
)

func tradeScenarioWorkflowFixture(t *testing.T, assessment risk.Assessment) (*Workflow,*PortfolioReview,*fakeMarketAgent) {
	t.Helper()
	market:=&fakeMarketAgent{result:validMarketContext()}
	graph,err:=NewArtifact(context.Background(),Config{
		MarketAgent:market,
		TechnicalAgent:&fakeTechnicalAgent{result:validTechnicalContext()},
		EventAgent:&fakeEventAgent{result:validEventContext()},
		Synthesizer:&fakeSynthesizer{},
	})
	if err!=nil {t.Fatal(err)}
	review,_,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(assessment))
	return graph,review,market
}

func tradeScenarioBook(input Input) *portfolio.Context {
	return &portfolio.Context{
		State:portfolio.StatePositions,
		AsOf:input.RequestedAsOf.Add(-2*time.Hour).Format(time.RFC3339),
		Positions:[]portfolio.Position{{Symbol:"600519.SH",Quantity:100}},
		Valuation:&portfolio.Valuation{
			Scope:portfolio.ValuationScopeCashAndPositions,
			Currency:portfolio.CurrencyCNY,
			TotalValueFen:100000,CashFen:40000,
			PositionValues:[]portfolio.PositionValue{
				{Symbol:"600519.SH",MarketValueFen:60000},
			},
		},
	}
}

func tradeScenarioPolicy(input Input) *portfolio.Limits {
	single:=int64(6500)
	invested:=int64(7500)
	return &portfolio.Limits{
		PolicyID:"scenario-policy",
		AsOf:input.RequestedAsOf.Add(-3*time.Hour).Format(time.RFC3339),
		Scope:portfolio.ValuationScopeCashAndPositions,
		Currency:portfolio.CurrencyCNY,
		Rules:[]portfolio.LimitRule{
			{Kind:portfolio.LimitMaxSinglePositionWeight,LimitBps:&single},
			{Kind:portfolio.LimitMaxInvestedWeight,LimitBps:&invested},
		},
	}
}

func tradeScenarioInput(input Input) *portfolio.TradeScenario {
	amount:=int64(10000)
	return &portfolio.TradeScenario{
		ScenarioID:"what-if-1",
		AsOf:input.RequestedAsOf.Add(-time.Hour).Format(time.RFC3339),
		Symbol:"600519.SH",Side:portfolio.ScenarioBuy,
		Currency:portfolio.CurrencyCNY,NotionalFen:&amount,
	}
}

func TestFullPortfolioWorkflowProjectsOnlyExplicitCandidateScenario(t *testing.T) {
	input:=validWorkflowInput()
	graph,review,market:=tradeScenarioWorkflowFixture(t,risk.AssessmentAcceptable)
	book:=tradeScenarioBook(input)
	result,err:=(&FullPortfolioWorkflow{
		Research:graph,Review:review,Limits:tradeScenarioPolicy(input),
		Scenario:tradeScenarioInput(input),
	}).Run(context.Background(),input,book)
	if err!=nil {t.Fatal(err)}
	if market.calls!=1 || result.TradeScenarioReview==nil ||
		result.TradeScenarioReview.Status!=portfolio.ScenarioBreach ||
		*result.TradeScenarioReview.ProjectedCashFen!=30000 ||
		result.TradeScenarioReview.Checks[0].ProjectedWeightBps!=7000 ||
		result.TradeScenarioReview.ExecutionAuthorized {
		t.Fatalf("only a research candidate can produce a modeled exposure: %+v",result.TradeScenarioReview)
	}
	if result.CurrentLimitsReview.Status!=portfolio.LimitReviewPass {
		t.Fatalf("original portfolio is within limits; scenario breached them: %+v",result.CurrentLimitsReview)
	}
	if result.PortfolioReview.PortfolioDecision.Disposition!=portfoliodecision.DispositionReviewRequired {
		t.Fatal("scenario must not authorize a trade")
	}
}

func TestFullPortfolioWorkflowMonitorProducesNoTradeProjection(t *testing.T) {
	input:=validWorkflowInput()
	graph,_,_:=tradeScenarioWorkflowFixture(t,risk.AssessmentAcceptable)
	review,_,_,trader,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentAcceptable))
	trader.intent.Status=tradeintent.StatusMonitor
	result,err:=(&FullPortfolioWorkflow{
		Research:graph, Review:review,
		Limits:tradeScenarioPolicy(input), Scenario:tradeScenarioInput(input),
	}).Run(context.Background(),input,tradeScenarioBook(input))
	if err!=nil {t.Fatal(err)}
	if result.TradeScenarioReview==nil ||
		result.TradeScenarioReview.Status!=portfolio.ScenarioNotApplicable ||
		result.TradeScenarioReview.ProjectedCashFen!=nil ||
		len(result.TradeScenarioReview.Checks)!=0 ||
		result.PortfolioReview.PortfolioDecision.Disposition!=portfoliodecision.DispositionNoAction {
		t.Fatalf("MONITOR must not produce projected limits or approvals: %+v",result)
	}
}

func TestFullPortfolioWorkflowRejectsInvalidScenarioBeforeAnyAgent(t *testing.T) {
	input:=validWorkflowInput()
	graph,review,market:=tradeScenarioWorkflowFixture(t,risk.AssessmentAcceptable)
	scenario:=tradeScenarioInput(input)
	scenario.Symbol="000001.SZ"
	result,err:=(&FullPortfolioWorkflow{
		Research:graph, Review:review, Limits:tradeScenarioPolicy(input),
		Scenario:scenario,
	}).Run(context.Background(),input,tradeScenarioBook(input))
	if err==nil || result!=nil || !strings.Contains(err.Error(),"does not match research symbol") {
		t.Fatalf("incorrect proposal symbol must fail: result=%+v error=%v",result,err)
	}
	if market.calls!=0 {t.Fatal("no model calls after malformed scenario")}
}

func TestFullPortfolioWorkflowScenarioNotProvidedStaysBackwardCompatible(t *testing.T) {
	input:=validWorkflowInput()
	graph,review,_:=tradeScenarioWorkflowFixture(t,risk.AssessmentHighRisk)
	result,err:=(&FullPortfolioWorkflow{Research:graph,Review:review}).
		Run(context.Background(),input,tradeScenarioBook(input))
	if err!=nil {t.Fatal(err)}
	if result.TradeScenarioReview!=nil {
		t.Fatalf("absent scenario must not generate hidden proposal: %+v",result.TradeScenarioReview)
	}
}

func TestFullPortfolioWorkflowCandidateWithoutMoneyCannotBeProjected(t *testing.T) {
	input:=validWorkflowInput()
	graph,_,_:=tradeScenarioWorkflowFixture(t,risk.AssessmentInsufficientContext)
	review,_,_,_,_:=reviewWorkflowFixture(t,&risk.Context{
		Symbol:"600519.SH", Assessment:risk.AssessmentInsufficientContext,
		Summary:"observed account not provided",Confidence:1,
	})
	book:=&portfolio.Context{State:portfolio.StateNotProvided}
	result,err:=(&FullPortfolioWorkflow{
		Research:graph,Review:review,Limits:tradeScenarioPolicy(input),
		Scenario:tradeScenarioInput(input),
	}).Run(context.Background(),input,book)
	if err!=nil {t.Fatal(err)}
	if result.TradeScenarioReview.Status!=portfolio.ScenarioInsufficientData ||
		result.TradeScenarioReview.ProjectedCashFen!=nil ||
		result.CurrentLimitsReview.Status!=portfolio.LimitReviewInsufficientData {
		t.Fatalf("unknown account must never produce hypothetical funded trade: %+v",result)
	}
}
