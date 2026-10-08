package research

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/portfoliodecision"
	"Aegis/internal/risk"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// This exercises the same production orchestration API used by cmd/aegis,
// rather than constructing a ResearchArtifact directly inside a hosted test.
func TestFullPortfolioWorkflowUsesOneResearchPass(t *testing.T) {
	marketAgent := &fakeMarketAgent{result: validMarketContext()}
	technicalAgent := &fakeTechnicalAgent{result: validTechnicalContext()}
	eventAgent := &fakeEventAgent{result: validEventContext()}
	synth := &fakeSynthesizer{}
	researchGraph, err := NewArtifact(context.Background(), Config{
		MarketAgent: marketAgent,
		TechnicalAgent: technicalAgent,
		EventAgent: eventAgent,
		Synthesizer: synth,
	})
	if err != nil {
		t.Fatal(err)
	}
	review, d, manager, trader, riskAnalyst := reviewWorkflowFixture(
		t, reviewContextFixture(risk.AssessmentHighRisk),
	)
	full := &FullPortfolioWorkflow{Research: researchGraph, Review: review}
	book := reviewBookFixture()
	book.AsOf = validWorkflowInput().RequestedAsOf.Format(time.RFC3339)
	result, err := full.Run(context.Background(), validWorkflowInput(), book)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ResearchArtifact == nil ||
		result.ResearchArtifact.Market != marketAgent.result ||
		result.ResearchArtifact.Technical != technicalAgent.result ||
		result.ResearchArtifact.Event != eventAgent.result {
		t.Fatalf("real upstream contexts not preserved: %+v", result.ResearchArtifact)
	}
	if marketAgent.calls != 1 || technicalAgent.calls != 1 ||
		eventAgent.calls != 1 || synth.calls != 1 ||
		d.calls != 1 || manager.calls != 1 || trader.calls != 1 ||
		riskAnalyst.calls != 1 || riskAnalyst.lastBook != book {
		t.Fatalf("unexpected call counts: market=%d tech=%d event=%d synth=%d debate=%d manager=%d trading=%d risk=%d",
			marketAgent.calls, technicalAgent.calls, eventAgent.calls,
			synth.calls, d.calls, manager.calls, trader.calls, riskAnalyst.calls)
	}
	if result.PortfolioReview.PortfolioDecision.Disposition != portfoliodecision.DispositionBlocked {
		t.Fatalf("HIGH_RISK must block: %+v", result.PortfolioReview.PortfolioDecision)
	}
	if err := portfoliodecision.Validate(
		result.PortfolioReview.PortfolioDecision,
		result.PortfolioReview.TradeIntent,
		book,
		result.PortfolioReview.RiskContext,
	); err != nil {
		t.Fatalf("invalid final portfolio policy: %v", err)
	}
}

func TestFullPortfolioWorkflowRejectsPortfolioBeforeExternalResearch(t *testing.T) {
	marketAgent := &fakeMarketAgent{result: validMarketContext()}
	researchGraph, err := NewArtifact(context.Background(), Config{
		MarketAgent: marketAgent,
		TechnicalAgent: &fakeTechnicalAgent{result: validTechnicalContext()},
		EventAgent: &fakeEventAgent{result: validEventContext()},
		Synthesizer: &fakeSynthesizer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	review, _, _, _, _ := reviewWorkflowFixture(t, reviewContextFixture(risk.AssessmentHighRisk))
	full := &FullPortfolioWorkflow{Research: researchGraph, Review: review}
	_, err = full.Run(context.Background(), validWorkflowInput(), nil)
	if err == nil || !strings.Contains(err.Error(), "portfolio context") {
		t.Fatalf("error = %v", err)
	}
	if marketAgent.calls != 0 {
		t.Fatalf("external research called before account validation: %d", marketAgent.calls)
	}
}

func TestFullPortfolioWorkflowStopsOnIncompleteEventResearch(t *testing.T) {
	marketAgent := &fakeMarketAgent{result: validMarketContext()}
	eventAgent := &fakeEventAgent{
		err: errors.New("semantic evidence research incomplete: missing match for raw document cls_news_2498111"),
	}
	researchGraph, err := NewArtifact(context.Background(), Config{
		MarketAgent: marketAgent,
		TechnicalAgent: &fakeTechnicalAgent{result: validTechnicalContext()},
		EventAgent: eventAgent,
		Synthesizer: &fakeSynthesizer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	review, d, manager, trader, riskAnalyst := reviewWorkflowFixture(
		t, reviewContextFixture(risk.AssessmentHighRisk),
	)
	full := &FullPortfolioWorkflow{Research: researchGraph, Review: review}
	book := reviewBookFixture()
	book.AsOf = validWorkflowInput().RequestedAsOf.Format(time.RFC3339)
	_, err = full.Run(context.Background(), validWorkflowInput(), book)
	if err == nil || !strings.Contains(err.Error(), "research incomplete") {
		t.Fatalf("missing evidence must fail, got %v", err)
	}
	if d.calls != 0 || manager.calls != 0 || trader.calls != 0 || riskAnalyst.calls != 0 {
		t.Fatalf("downstream agents must not run after incomplete evidence: %d/%d/%d/%d",
			d.calls, manager.calls, trader.calls, riskAnalyst.calls)
	}
}

func TestNewArtifactDoesNotRequireTradingAgent(t *testing.T) {
	graph, err := NewArtifact(context.Background(), Config{
		MarketAgent: &fakeMarketAgent{result: validMarketContext()},
		TechnicalAgent: &fakeTechnicalAgent{result: validTechnicalContext()},
		EventAgent: &fakeEventAgent{result: validEventContext()},
		Synthesizer: &fakeSynthesizer{},
	})
	if err != nil {
		t.Fatalf("NewArtifact() error = %v", err)
	}
	artifact, err := graph.RunArtifact(context.Background(), validWorkflowInput())
	if err != nil {
		t.Fatalf("RunArtifact() error = %v", err)
	}
	if artifact.Symbol != "600519.SH" {
		t.Fatalf("wrong artifact symbol: %+v", artifact)
	}
	if _, err := graph.Run(context.Background(), validWorkflowInput()); err == nil {
		t.Fatal("artifact workflow must not secretly run a trading agent")
	}
}

func TestFullPortfolioWorkflowUnknownIsNotEmpty(t *testing.T) {
	// This is explicitly unknown account data, not an observed empty account.
	book := &portfolio.Context{State: portfolio.StateNotProvided}
	if err := portfolio.Validate(book); err != nil {
		t.Fatal(err)
	}
	// The true production path receives this exact state unchanged.
	review, _, _, _, riskAnalyst := reviewWorkflowFixture(t, &risk.Context{
		Symbol:"600519.SH",
		Assessment:risk.AssessmentInsufficientContext,
		Summary:"account state has not been provided",
		Confidence:1,
	})
	upstream,err:=NewArtifact(context.Background(),Config{
		MarketAgent:&fakeMarketAgent{result:validMarketContext()},
		TechnicalAgent:&fakeTechnicalAgent{result:validTechnicalContext()},
		EventAgent:&fakeEventAgent{result:validEventContext()},
		Synthesizer:&fakeSynthesizer{},
	})
	if err!=nil { t.Fatal(err) }
	result,err:=(&FullPortfolioWorkflow{Research:upstream,Review:review}).Run(
		context.Background(),validWorkflowInput(),book)
	if err!=nil { t.Fatalf("Run() error = %v",err) }
	if riskAnalyst.lastBook!=book || result.PortfolioReview.RiskContext.Assessment!=risk.AssessmentInsufficientContext {
		t.Fatalf("unknown portfolio was lost: %+v", result.PortfolioReview)
	}
}

func TestFullPortfolioWorkflowRejectsFutureAccountStateBeforeResearch(t *testing.T) {
	marketAgent:=&fakeMarketAgent{result:validMarketContext()}
	graph,err:=NewArtifact(context.Background(),Config{
		MarketAgent:marketAgent,
		TechnicalAgent:&fakeTechnicalAgent{result:validTechnicalContext()},
		EventAgent:&fakeEventAgent{result:validEventContext()},
		Synthesizer:&fakeSynthesizer{},
	})
	if err!=nil {t.Fatal(err)}
	review,_,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
	book:=reviewBookFixture() // October snapshot, September research cutoff.
	_,err=(&FullPortfolioWorkflow{Research:graph, Review:review}).Run(
		context.Background(),validWorkflowInput(),book)
	if err==nil || !strings.Contains(err.Error(),"after research requested_as_of") {
		t.Fatalf("error = %v",err)
	}
	if marketAgent.calls!=0 {t.Fatal("future account state must fail before live data retrieval")}
}

func TestFullPortfolioWorkflowRejectsStaleSnapshotBeforeAgents(t *testing.T) {
	marketAgent := &fakeMarketAgent{result: validMarketContext()}
	techAgent := &fakeTechnicalAgent{result: validTechnicalContext()}
	eventAgent := &fakeEventAgent{result: validEventContext()}
	synth := &fakeSynthesizer{}
	graph, err := NewArtifact(context.Background(), Config{
		MarketAgent: marketAgent,
		TechnicalAgent: techAgent,
		EventAgent: eventAgent,
		Synthesizer: synth,
	})
	if err != nil { t.Fatal(err) }
	review, debateAgent, managerAgent, trader, riskAnalyst :=
		reviewWorkflowFixture(t, reviewContextFixture(risk.AssessmentHighRisk))
	book := reviewBookFixture()
	book.AsOf = validWorkflowInput().RequestedAsOf.Add(-48*time.Hour).Format(time.RFC3339)
	w := &FullPortfolioWorkflow{
		Research: graph, Review: review, MaxPortfolioAge: 24*time.Hour,
	}
	result, err := w.Run(context.Background(),validWorkflowInput(),book)
	if err==nil || !strings.Contains(err.Error(),"portfolio snapshot is stale") || result!=nil {
		t.Fatalf("expected stale snapshot to block review: result=%+v error=%v",result,err)
	}
	if marketAgent.calls!=0 || techAgent.calls!=0 || eventAgent.calls!=0 ||
		synth.calls!=0 || debateAgent.calls!=0 || managerAgent.calls!=0 ||
		trader.calls!=0 || riskAnalyst.calls!=0 {
		t.Fatalf("stale account must be rejected before every research/review agent")
	}
}

func TestFullPortfolioWorkflowFreshnessPolicyIsMeasuredAtResearchCutoff(t *testing.T) {
	graph,err:=NewArtifact(context.Background(),Config{
		MarketAgent:&fakeMarketAgent{result:validMarketContext()},
		TechnicalAgent:&fakeTechnicalAgent{result:validTechnicalContext()},
		EventAgent:&fakeEventAgent{result:validEventContext()},
		Synthesizer:&fakeSynthesizer{},
	})
	if err!=nil {t.Fatal(err)}
	review,_,_,_,_:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
	book:=reviewBookFixture()
	book.AsOf=validWorkflowInput().RequestedAsOf.Add(-12*time.Hour).Format(time.RFC3339)
	result,err:=(&FullPortfolioWorkflow{
		Research:graph, Review:review,MaxPortfolioAge:24*time.Hour,
	}).Run(context.Background(),validWorkflowInput(),book)
	if err!=nil || result==nil {
		t.Fatalf("historical PIT snapshot within explicit age policy must pass: result=%+v error=%v",result,err)
	}
}
