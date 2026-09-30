package research

import (
	eventagent "Aegis/internal/agent/event"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/agent/trading"
	eventcap "Aegis/internal/capability/event"
	"Aegis/internal/decision"
	"Aegis/internal/research/synthesis"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
)

type fakeMarketAgent struct {
	query   string
	result  *market.Context
	err     error
	calls   int
	started chan<- struct{}
	release <-chan struct{}
}

func (a *fakeMarketAgent) Analyze(
	_ context.Context,
	query string,
	_ ...adk.AgentRunOption,
) (*market.Context, error) {
	a.calls++
	a.query = query
	if a.started != nil {
		a.started <- struct{}{}
	}
	if a.release != nil {
		<-a.release
	}
	if a.err != nil {
		return nil, a.err
	}
	return a.result, nil
}

type fakeTechnicalAgent struct {
	input  technical.Input
	result *technical.Context
	err    error
	calls  int
}

func (a *fakeTechnicalAgent) Analyze(
	_ context.Context,
	input technical.Input,
	_ ...adk.AgentRunOption,
) (*technical.Context, error) {
	a.calls++
	a.input = input
	if a.err != nil {
		return nil, a.err
	}
	return a.result, nil
}

type fakeEventAgent struct {
	input   eventagent.Input
	result  *eventagent.Context
	err     error
	calls   int
	started chan<- struct{}
	release <-chan struct{}
}

func (a *fakeEventAgent) Analyze(
	_ context.Context,
	input eventagent.Input,
	_ ...adk.AgentRunOption,
) (*eventagent.Context, error) {
	a.calls++
	a.input = input
	if a.started != nil {
		a.started <- struct{}{}
	}
	if a.release != nil {
		<-a.release
	}
	if a.err != nil {
		return nil, a.err
	}
	return a.result, nil
}

type fakeSynthesizer struct {
	input  synthesis.Input
	result *synthesis.Artifact
	err    error
	calls  int
}

func (s *fakeSynthesizer) Synthesize(
	_ context.Context,
	input synthesis.Input,
	_ ...model.Option,
) (*synthesis.Artifact, error) {
	s.calls++
	s.input = input
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &synthesis.Artifact{
		Symbol:    input.Symbol,
		Market:    input.Market,
		Technical: input.Technical,
		Event:     input.Event,
		Synthesis: synthesis.Synthesis{
			Summary:    "cross-domain synthesis",
			Confidence: 0.8,
		},
	}, nil
}

type fakeArtifactTradingAgent struct {
	artifact *synthesis.Artifact
	result   *decision.Decision
	err      error
	calls    int

	legacyInput trading.Input
	legacyCalls int
}

func (a *fakeArtifactTradingAgent) AnalyzeArtifact(
	_ context.Context,
	artifact *synthesis.Artifact,
	_ ...adk.AgentRunOption,
) (*decision.Decision, error) {
	a.calls++
	a.artifact = artifact
	if a.err != nil {
		return nil, a.err
	}
	return a.result, nil
}

func (a *fakeArtifactTradingAgent) Analyze(
	_ context.Context,
	input trading.Input,
	_ ...adk.AgentRunOption,
) (*decision.Decision, error) {
	a.legacyCalls++
	a.legacyInput = input
	if a.err != nil {
		return nil, a.err
	}
	return a.result, nil
}

func validMarketContext() *market.Context {
	return &market.Context{
		AsOf:       "20260923",
		Regime:     market.RegimeMixed,
		Evidence:   []string{"市场分化"},
		Risk:       []string{"广度偏弱"},
		Confidence: 0.64,
	}
}

func validTechnicalContext() *technical.Context {
	return &technical.Context{
		Symbol:        "600519.SH",
		RequestedAsOf: "20260923",
		EffectiveAsOf: "20260923",
		Status:        technical.StatusNormal,
		Snapshot: &technical.Snapshot{
			Close: 1251.24,
		},
		Evidence:   []string{"收盘价低于主要均线"},
		Risk:       []string{"短期弱势需确认"},
		Confidence: 0.82,
	}
}

func validEventContext() *eventagent.Context {
	return &eventagent.Context{
		Symbol:        "600519.SH",
		RequestedAsOf: "2026-09-23T15:00:00+08:00",
		Status:        eventagent.StatusNoEvent,
		Confidence:    0.88,
	}
}

func validDecision() *decision.Decision {
	return &decision.Decision{
		Symbol:     "600519.SH",
		MarketView: "研究整合后维持观察",
		Evidence:   []string{"ResearchArtifact 已完成跨域整合"},
		Risk:       []string{"仍需等待技术确认"},
		Action:     decision.ActionWatch,
		Confidence: 0.66,
	}
}

func validWorkflowInput() Input {
	asOf := time.Date(
		2026, 9, 23, 15, 0, 0, 0,
		time.FixedZone("Asia/Shanghai", 8*60*60),
	)
	return Input{
		Symbol:        "600519.SH",
		RequestedAsOf: asOf,
		EventInput: eventagent.Input{
			Task: eventcap.ResearchTask{
				ID:              "event-research",
				Symbol:          "600519.SH",
				Objective:       "研究标的近期事件",
				InformationNeed: eventcap.NeedEventEvidence,
				AsOf:            asOf,
				Requirements: eventcap.ResearchRequirements{
					PITSafe: true,
				},
			},
			Query: "研究近期公司事件",
		},
	}
}

func TestWorkflowRunUsesResearchArtifactBoundary(
	t *testing.T,
) {
	marketContext := validMarketContext()
	technicalContext := validTechnicalContext()
	eventContext := validEventContext()

	marketAgent := &fakeMarketAgent{
		result: marketContext,
	}
	technicalAgent := &fakeTechnicalAgent{
		result: technicalContext,
	}
	eventAgent := &fakeEventAgent{
		result: eventContext,
	}
	synthesizer := &fakeSynthesizer{}
	tradingAgent := &fakeArtifactTradingAgent{
		result: validDecision(),
	}

	workflow, err := New(
		context.Background(),
		Config{
			MarketAgent:    marketAgent,
			TechnicalAgent: technicalAgent,
			EventAgent:     eventAgent,
			Synthesizer:    synthesizer,
			TradingAgent:   tradingAgent,
		},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := workflow.Run(
		context.Background(),
		validWorkflowInput(),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Symbol != "600519.SH" {
		t.Fatalf(
			"decision symbol = %q",
			result.Symbol,
		)
	}

	if marketAgent.calls != 1 ||
		technicalAgent.calls != 1 ||
		eventAgent.calls != 1 ||
		synthesizer.calls != 1 ||
		tradingAgent.calls != 1 {
		t.Fatalf(
			"calls market=%d technical=%d event=%d synthesis=%d trading=%d",
			marketAgent.calls,
			technicalAgent.calls,
			eventAgent.calls,
			synthesizer.calls,
			tradingAgent.calls,
		)
	}

	if technicalAgent.input.Symbol != "600519.SH" ||
		technicalAgent.input.RequestedAsOf !=
			"20260923" {
		t.Fatalf(
			"technical input = %+v",
			technicalAgent.input,
		)
	}
	if synthesizer.input.Market != marketContext ||
		synthesizer.input.Technical != technicalContext ||
		synthesizer.input.Event != eventContext {
		t.Fatalf(
			"synthesis input did not preserve exact contexts: %+v",
			synthesizer.input,
		)
	}
	if tradingAgent.artifact == nil ||
		tradingAgent.artifact != synthesizer.result &&
			synthesizer.result != nil {
		t.Fatalf(
			"trading artifact = %+v",
			tradingAgent.artifact,
		)
	}
	if tradingAgent.artifact.Market != marketContext ||
		tradingAgent.artifact.Technical != technicalContext ||
		tradingAgent.artifact.Event != eventContext {
		t.Fatal(
			"main trading path did not receive the complete ResearchArtifact",
		)
	}
	if tradingAgent.legacyCalls != 0 {
		t.Fatalf(
			"legacy trading calls = %d, want 0",
			tradingAgent.legacyCalls,
		)
	}
}

func TestWorkflowMarketAndEventBranchesRunConcurrently(
	t *testing.T,
) {
	marketStarted := make(chan struct{}, 1)
	eventStarted := make(chan struct{}, 1)
	release := make(chan struct{})

	workflow, err := New(
		context.Background(),
		Config{
			MarketAgent: &fakeMarketAgent{
				result:  validMarketContext(),
				started: marketStarted,
				release: release,
			},
			TechnicalAgent: &fakeTechnicalAgent{
				result: validTechnicalContext(),
			},
			EventAgent: &fakeEventAgent{
				result:  validEventContext(),
				started: eventStarted,
				release: release,
			},
			Synthesizer: &fakeSynthesizer{},
			TradingAgent: &fakeArtifactTradingAgent{
				result: validDecision(),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, runErr := workflow.Run(
			context.Background(),
			validWorkflowInput(),
		)
		done <- runErr
	}()

	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()

	marketSeen := false
	eventSeen := false
	for !marketSeen || !eventSeen {
		select {
		case <-marketStarted:
			marketSeen = true
		case <-eventStarted:
			eventSeen = true
		case <-timeout.C:
			close(release)
			t.Fatalf(
				"market/event did not overlap: market=%v event=%v",
				marketSeen,
				eventSeen,
			)
		}
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestWorkflowSynthesisWaitsForBothResearchBranches(
	t *testing.T,
) {
	marketContext := validMarketContext()
	technicalContext := validTechnicalContext()
	eventContext := validEventContext()

	technicalRelease := make(chan struct{})
	eventRelease := make(chan struct{})
	synthesisCalled := make(chan struct{}, 1)

	technicalAgent := &blockingTechnicalAgent{
		result:  technicalContext,
		started: make(chan struct{}),
		release: technicalRelease,
	}
	eventAgent := &blockingEventAgent{
		result:  eventContext,
		started: make(chan struct{}),
		release: eventRelease,
	}
	synthesizer := &signalingSynthesizer{
		fakeSynthesizer: fakeSynthesizer{},
		called:          synthesisCalled,
	}

	workflow, err := New(
		context.Background(),
		Config{
			MarketAgent: &fakeMarketAgent{
				result: marketContext,
			},
			TechnicalAgent: technicalAgent,
			EventAgent:     eventAgent,
			Synthesizer:    synthesizer,
			TradingAgent: &fakeArtifactTradingAgent{
				result: validDecision(),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, runErr := workflow.Run(
			context.Background(),
			validWorkflowInput(),
		)
		done <- runErr
	}()

	<-technicalAgent.started
	<-eventAgent.started

	close(technicalRelease)
	select {
	case <-synthesisCalled:
		t.Fatal(
			"synthesis ran before event branch completed",
		)
	case <-time.After(100 * time.Millisecond):
	}

	close(eventRelease)
	select {
	case <-synthesisCalled:
	case <-time.After(2 * time.Second):
		t.Fatal(
			"synthesis did not run after both branches completed",
		)
	}

	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

type blockingTechnicalAgent struct {
	result  *technical.Context
	started chan struct{}
	release <-chan struct{}
}

func (a *blockingTechnicalAgent) Analyze(
	_ context.Context,
	_ technical.Input,
	_ ...adk.AgentRunOption,
) (*technical.Context, error) {
	if a.started == nil {
		a.started = make(chan struct{})
	}
	close(a.started)
	<-a.release
	return a.result, nil
}

type blockingEventAgent struct {
	result  *eventagent.Context
	started chan struct{}
	release <-chan struct{}
}

func (a *blockingEventAgent) Analyze(
	_ context.Context,
	_ eventagent.Input,
	_ ...adk.AgentRunOption,
) (*eventagent.Context, error) {
	if a.started == nil {
		a.started = make(chan struct{})
	}
	close(a.started)
	<-a.release
	return a.result, nil
}

type signalingSynthesizer struct {
	fakeSynthesizer
	called chan<- struct{}
}

func (s *signalingSynthesizer) Synthesize(
	ctx context.Context,
	input synthesis.Input,
	opts ...model.Option,
) (*synthesis.Artifact, error) {
	s.called <- struct{}{}
	return s.fakeSynthesizer.Synthesize(
		ctx,
		input,
		opts...,
	)
}

func TestWorkflowRejectsEventAsOfMismatchBeforeGraphWork(t *testing.T) {
	marketAgent := &fakeMarketAgent{result: validMarketContext()}
	eventAgent := &fakeEventAgent{result: validEventContext()}
	workflow, err := New(context.Background(), Config{
		MarketAgent: marketAgent,
		TechnicalAgent: &fakeTechnicalAgent{result: validTechnicalContext()},
		EventAgent: eventAgent,
		Synthesizer: &fakeSynthesizer{},
		TradingAgent: &fakeArtifactTradingAgent{result: validDecision()},
	})
	if err != nil { t.Fatal(err) }
	input := validWorkflowInput()
	input.EventInput.Task.AsOf = input.RequestedAsOf.Add(-time.Second)
	_, err = workflow.Run(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "does not match research requested_as_of") {
		t.Fatalf("error = %v, want as_of mismatch", err)
	}
	if marketAgent.calls != 0 || eventAgent.calls != 0 {
		t.Fatalf("graph work started before input rejection: market=%d event=%d", marketAgent.calls, eventAgent.calls)
	}
}

func TestWorkflowRejectsEventTaskSymbolMismatchBeforeGraphWork(
	t *testing.T,
) {
	marketAgent := &fakeMarketAgent{
		result: validMarketContext(),
	}
	eventAgent := &fakeEventAgent{
		result: validEventContext(),
	}

	workflow, err := New(
		context.Background(),
		Config{
			MarketAgent: marketAgent,
			TechnicalAgent: &fakeTechnicalAgent{
				result: validTechnicalContext(),
			},
			EventAgent:  eventAgent,
			Synthesizer: &fakeSynthesizer{},
			TradingAgent: &fakeArtifactTradingAgent{
				result: validDecision(),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	input := validWorkflowInput()
	input.EventInput.Task.Symbol = "000001.SZ"

	_, err = workflow.Run(
		context.Background(),
		input,
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"does not match research symbol",
		) {
		t.Fatalf(
			"error = %v, want symbol mismatch",
			err,
		)
	}
	if marketAgent.calls != 0 ||
		eventAgent.calls != 0 {
		t.Fatalf(
			"graph work started before input rejection: market=%d event=%d",
			marketAgent.calls,
			eventAgent.calls,
		)
	}
}

func TestWorkflowEventErrorStopsFanInAndTrading(
	t *testing.T,
) {
	synthesizer := &fakeSynthesizer{}
	tradingAgent := &fakeArtifactTradingAgent{
		result: validDecision(),
	}
	workflow, err := New(
		context.Background(),
		Config{
			MarketAgent: &fakeMarketAgent{
				result: validMarketContext(),
			},
			TechnicalAgent: &fakeTechnicalAgent{
				result: validTechnicalContext(),
			},
			EventAgent: &fakeEventAgent{
				err: errors.New(
					"event failed",
				),
			},
			Synthesizer:  synthesizer,
			TradingAgent: tradingAgent,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = workflow.Run(
		context.Background(),
		validWorkflowInput(),
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"event research",
		) {
		t.Fatalf(
			"error = %v, want event research error",
			err,
		)
	}
	if synthesizer.calls != 0 {
		t.Fatalf(
			"synthesis calls = %d, want 0",
			synthesizer.calls,
		)
	}
	if tradingAgent.calls != 0 {
		t.Fatalf(
			"trading calls = %d, want 0",
			tradingAgent.calls,
		)
	}
}

func TestNewRequiresMainGraphDependencies(
	t *testing.T,
) {
	base := Config{
		MarketAgent: &fakeMarketAgent{},
		TechnicalAgent: &fakeTechnicalAgent{},
		EventAgent:     &fakeEventAgent{},
		Synthesizer:    &fakeSynthesizer{},
		TradingAgent:   &fakeArtifactTradingAgent{},
	}

	tests := []struct {
		name string
		edit func(*Config)
	}{
		{
			name: "market",
			edit: func(c *Config) {
				c.MarketAgent = nil
			},
		},
		{
			name: "technical",
			edit: func(c *Config) {
				c.TechnicalAgent = nil
			},
		},
		{
			name: "event",
			edit: func(c *Config) {
				c.EventAgent = nil
			},
		},
		{
			name: "synthesis",
			edit: func(c *Config) {
				c.Synthesizer = nil
			},
		},
		{
			name: "trading",
			edit: func(c *Config) {
				c.TradingAgent = nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			test.edit(&cfg)
			if _, err := New(
				context.Background(),
				cfg,
			); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewLegacyPreservesPreEventWorkflow(
	t *testing.T,
) {
	marketContext := validMarketContext()
	technicalContext := validTechnicalContext()
	tradingAgent := &fakeArtifactTradingAgent{
		result: validDecision(),
	}

	workflow, err := NewLegacy(
		context.Background(),
		LegacyConfig{
			MarketAgent: &fakeMarketAgent{
				result: marketContext,
			},
			TechnicalAgent: &fakeTechnicalAgent{
				result: technicalContext,
			},
			TradingAgent: tradingAgent,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := workflow.Run(
		context.Background(),
		Input{Symbol: "600519.SH"},
	)
	if err != nil {
		t.Fatalf(
			"legacy Run() error = %v",
			err,
		)
	}
	if result.Symbol != "600519.SH" {
		t.Fatalf(
			"result = %+v",
			result,
		)
	}
	if tradingAgent.legacyCalls != 1 ||
		tradingAgent.calls != 0 {
		t.Fatalf(
			"legacy=%d artifact=%d",
			tradingAgent.legacyCalls,
			tradingAgent.calls,
		)
	}
	if tradingAgent.legacyInput.MarketContext !=
		marketContext ||
		tradingAgent.legacyInput.TechnicalContext !=
			technicalContext {
		t.Fatal(
			"legacy contexts were not preserved",
		)
	}
}
