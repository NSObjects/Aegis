package portfolio

import (
	"testing"
	"time"
)

func accountResearchFixture() (ResearchAccountSnapshot, ResearchRiskPolicy, time.Time) {
	cutoff := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	asOf := cutoff.Add(-time.Hour)
	cost, limit, budget := int64(0), int64(5000), int64(1000)
	source := ResearchSource{Kind: ResearchSourceOperator, Reference: "statement/20261119", AvailableAt: cutoff}
	return ResearchAccountSnapshot{
			AccountID: "account-1", Version: "snapshot-7", Source: &source,
			Book: Context{
				State: StatePositions, AsOf: asOf.Format(time.RFC3339),
				Positions: []Position{{Symbol: "600519.SH", Quantity: 100}, {Symbol: "000001.SZ", Quantity: 200}},
				Valuation: &Valuation{
					Scope: ValuationScopeCashAndPositions, Currency: CurrencyCNY, CashFen: 20_000, TotalValueFen: 100_000,
					PositionValues: []PositionValue{{Symbol: "600519.SH", MarketValueFen: 60_000}, {Symbol: "000001.SZ", MarketValueFen: 20_000}},
				},
			},
			Costs: []PositionCost{{Symbol: "600519.SH", CostFen: &cost}},
		}, ResearchRiskPolicy{
			Version: "policy-v3", Source: ResearchSource{Kind: ResearchSourceOperator, Reference: "risk-policy/3", AvailableAt: asOf},
			Limits: Limits{PolicyID: "research-risk", AsOf: asOf.Format(time.RFC3339), Scope: ValuationScopeCashAndPositions,
				Currency: CurrencyCNY, Rules: []LimitRule{{Kind: LimitMaxSinglePositionWeight, LimitBps: &limit}}},
			LossBudgetFen: &budget,
		}, cutoff
}

func TestResearchAccountSeparatesObservedValuesAndPolicy(t *testing.T) {
	snapshot, policy, cutoff := accountResearchFixture()
	view, err := AssessResearchAccount(snapshot, &policy, cutoff, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if view.AccountVersion != snapshot.Version || view.PolicyVersion != policy.Version ||
		view.Exposure == nil || view.Exposure.CashFen != 20_000 || view.Exposure.InvestedValueFen != 80_000 ||
		view.LimitReview.Status != LimitReviewBreach || view.LossBudgetFen == nil || *view.LossBudgetFen != 1000 {
		t.Fatalf("unexpected observed and policy outputs: %+v", view)
	}
	if view.Positions[0].CostFen == nil || *view.Positions[0].CostFen != 0 || view.Positions[1].CostFen != nil {
		t.Fatalf("unknown and zero costs were conflated: %+v", view.Positions)
	}
	*snapshot.Costs[0].CostFen = 100
	*policy.LossBudgetFen = 100
	if *view.Positions[0].CostFen != 0 || *view.LossBudgetFen != 1000 {
		t.Fatal("research view mutated with inputs")
	}
}

func TestResearchAccountPreservesUnknownAndExplicitZero(t *testing.T) {
	snapshot, policy, cutoff := accountResearchFixture()
	absent := ResearchAccountSnapshot{Book: Context{State: StateNotProvided}}
	view, err := AssessResearchAccount(absent, &policy, cutoff, time.Hour)
	if err != nil || view.Exposure != nil || view.AccountSource != nil ||
		view.LimitReview == nil || view.LimitReview.Status != LimitReviewInsufficientData {
		t.Fatalf("unknown account yielded observed values: %+v, %v", view, err)
	}
	snapshot.Book = Context{State: StateEmpty, AsOf: cutoff.Format(time.RFC3339), Valuation: &Valuation{
		Scope: ValuationScopeCashAndPositions, Currency: CurrencyCNY, PositionValues: []PositionValue{},
	}}
	snapshot.Costs = nil
	view, err = AssessResearchAccount(snapshot, nil, cutoff, time.Hour)
	if err != nil || view.Exposure == nil || view.Exposure.TotalValueFen != 0 || view.Exposure.CashWeightBps != nil ||
		view.LossBudgetFen != nil {
		t.Fatalf("explicit zero valuation or missing policy was lost: %+v, %v", view, err)
	}
}

func TestResearchAccountRejectsUnreconciledOrUnavailableInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*ResearchAccountSnapshot, *ResearchRiskPolicy, time.Time)
	}{
		{"future observation", func(s *ResearchAccountSnapshot, _ *ResearchRiskPolicy, cutoff time.Time) {
			s.Source.AvailableAt = cutoff.Add(time.Second)
		}},
		{"future policy", func(_ *ResearchAccountSnapshot, p *ResearchRiskPolicy, cutoff time.Time) {
			p.Source.AvailableAt = cutoff.Add(time.Second)
		}},
		{"unreconciled value", func(s *ResearchAccountSnapshot, _ *ResearchRiskPolicy, _ time.Time) {
			s.Book.Valuation.TotalValueFen++
		}},
		{"unmatched cost", func(s *ResearchAccountSnapshot, _ *ResearchRiskPolicy, _ time.Time) {
			s.Costs[0].Symbol = "OTHER"
		}},
		{"unversioned snapshot", func(s *ResearchAccountSnapshot, _ *ResearchRiskPolicy, _ time.Time) {
			s.Version = ""
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, policy, cutoff := accountResearchFixture()
			tc.change(&snapshot, &policy, cutoff)
			if _, err := AssessResearchAccount(snapshot, &policy, cutoff, time.Hour); err == nil {
				t.Fatal("invalid account research input was accepted")
			}
		})
	}
	snapshot, policy, cutoff := accountResearchFixture()
	if _, err := AssessResearchAccount(snapshot, &policy, cutoff, time.Minute); err == nil {
		t.Fatal("stale account was accepted")
	}
}
