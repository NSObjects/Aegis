package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"Aegis/internal/decision"
	"Aegis/internal/portfolio"
	"Aegis/internal/research"
)

type fakeDecisionRunner struct {
	calls  int
	input  research.Input
	result *decision.Decision
	err    error
}

func (f *fakeDecisionRunner) Run(_ context.Context, input research.Input) (*decision.Decision, error) {
	f.calls++
	f.input = input
	return f.result, f.err
}

type fakePortfolioRunner struct {
	calls  int
	input  research.Input
	book   *portfolio.Context
	result *research.FullPortfolioResult
	err    error
}

func (f *fakePortfolioRunner) Run(_ context.Context, input research.Input, book *portfolio.Context) (*research.FullPortfolioResult, error) {
	f.calls++
	f.input = input
	f.book = book
	return f.result, f.err
}

func TestResearchServiceDecisionKeepsTypedLegacyOutput(t *testing.T) {
	decisionResult := &decision.Decision{Symbol: "600519.SH", Action: decision.ActionWatch, MarketView: "mock", Evidence: []string{"mock"}}
	dec := &fakeDecisionRunner{result: decisionResult}
	port := &fakePortfolioRunner{result: &research.FullPortfolioResult{}}
	svc, err := NewResearchService(dec, port)
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 10, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	input := research.Input{Symbol: "600519.SH", RequestedAsOf: asOf}
	result, err := svc.Run(context.Background(), ResearchRequest{Mode: ModeDecision, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != ModeDecision || result.Decision != decisionResult || result.Portfolio != nil {
		t.Fatal("unexpected result envelope")
	}
	if dec.calls != 1 || port.calls != 0 || dec.input.RequestedAsOf != asOf {
		t.Fatal("workflow called incorrectly")
	}
	got, err := json.Marshal(result.Output())
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(decisionResult)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("CLI output changed: %s != %s", got, want)
	}
}

func TestResearchServiceFullReviewPreservesAccountAndResult(t *testing.T) {
	dec := &fakeDecisionRunner{}
	payload := &research.FullPortfolioResult{}
	port := &fakePortfolioRunner{result: payload}
	svc, _ := NewResearchService(dec, port)
	book := &portfolio.Context{State: portfolio.StateNotProvided}
	input := research.Input{Symbol: "600519.SH", RequestedAsOf: time.Now()}
	got, err := svc.Run(context.Background(), ResearchRequest{Mode: ModePortfolio, Input: input, Portfolio: book})
	if err != nil {
		t.Fatal(err)
	}
	if got.Output() != payload || got.Decision != nil || port.book != book || port.input.Symbol != input.Symbol || port.calls != 1 || dec.calls != 0 {
		t.Fatal("full review was not forwarded exactly once")
	}
}

func TestResearchServiceRejectsInvalidRequestsBeforeAnyProvider(t *testing.T) {
	d := &fakeDecisionRunner{result: &decision.Decision{}}
	p := &fakePortfolioRunner{result: &research.FullPortfolioResult{}}
	svc, _ := NewResearchService(d, p)
	cases := []struct {
		name string
		ctx  context.Context
		req  ResearchRequest
		want string
	}{
		{"unknown", context.Background(), ResearchRequest{Mode: "WRONG"}, "unsupported research mode"},
		{"nil account", context.Background(), ResearchRequest{Mode: ModePortfolio}, "explicit account snapshot"},
		{"account in decision", context.Background(), ResearchRequest{Mode: ModeDecision, Portfolio: &portfolio.Context{State: portfolio.StateEmpty}}, "cannot accept"},
		{"nil context", nil, ResearchRequest{Mode: ModeDecision}, "context is nil"},
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	cases = append(cases, struct {
		name string
		ctx  context.Context
		req  ResearchRequest
		want string
	}{"canceled", canceled, ResearchRequest{Mode: ModeDecision}, "canceled"})
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r, err := svc.Run(tt.ctx, tt.req)
			if r != nil || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %#v, %v, want %s", r, err, tt.want)
			}
		})
	}
	if d.calls != 0 || p.calls != 0 {
		t.Fatalf("invalid request reached runner: %d %d", d.calls, p.calls)
	}
	empty, err := NewResearchService(nil, nil)
	if empty != nil || err == nil {
		t.Fatal("empty service should fail")
	}
	var nilService *ResearchService
	if _, err := nilService.Run(context.Background(), ResearchRequest{}); err == nil {
		t.Fatal("nil receiver should fail")
	}
}

func TestResearchServicePropagatesErrorsAndRejectsNilOutputs(t *testing.T) {
	marker := errors.New("provider unavailable")
	d := &fakeDecisionRunner{err: marker}
	p := &fakePortfolioRunner{result: nil}
	svc, _ := NewResearchService(d, p)
	_, err := svc.Run(context.Background(), ResearchRequest{Mode: ModeDecision})
	if !errors.Is(err, marker) {
		t.Fatalf("lost wrapped error: %v", err)
	}
	d.err = nil
	if r, e := svc.Run(context.Background(), ResearchRequest{Mode: ModeDecision}); r != nil || e == nil {
		t.Fatalf("nil decision accepted: %#v %v", r, e)
	}
	if r, e := svc.Run(context.Background(), ResearchRequest{Mode: ModePortfolio, Portfolio: &portfolio.Context{State: portfolio.StateNotProvided}}); r != nil || e == nil {
		t.Fatalf("nil portfolio accepted: %#v %v", r, e)
	}
	if (&ResearchResponse{}).Output() != nil || (*ResearchResponse)(nil).Output() != nil {
		t.Fatal("invalid responses must not expose data")
	}
}
