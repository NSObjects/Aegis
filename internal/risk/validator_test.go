package risk

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/tradeintent"
	"strings"
	"testing"
)

func validIntent() *tradeintent.Intent {
	return &tradeintent.Intent{
		Symbol:     "600519.SH",
		Thesis:     "research thesis",
		Evidence:   []string{"grounded evidence"},
		Risk:       []string{"known risk"},
		Status:     tradeintent.StatusCandidate,
		Confidence: 0.68,
	}
}

func emptyPortfolio() *portfolio.Context {
	return &portfolio.Context{
		State: portfolio.StateEmpty,
		AsOf:  "2026-10-08T01:30:00+08:00",
	}
}

func positionsPortfolio() *portfolio.Context {
	return &portfolio.Context{
		State: portfolio.StatePositions,
		AsOf:  "2026-10-08T01:30:00+08:00",
		Positions: []portfolio.Position{
			{Symbol: "600519.SH", Quantity: 100},
			{Symbol: "000001.SZ", Quantity: 200},
		},
	}
}

func TestValidateRiskContext(t *testing.T) {
	ctx := &Context{
		Symbol:     "600519.SH",
		Assessment: AssessmentCaution,
		Summary:    "candidate is reviewable but portfolio exposure requires caution",
		Findings: []Item{
			{
				Claim: "trade intent contains known downside risk",
				References: []Reference{
					{Domain: ReferenceTradeIntent},
				},
			},
		},
		Constraints: []Item{
			{
				Claim: "existing position should be considered before any portfolio action",
				References: []Reference{
					{Domain: ReferencePortfolio, Symbol: "600519.SH"},
				},
			},
		},
		Confidence: 0.72,
	}
	if err := Validate(ctx, validIntent(), positionsPortfolio()); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateNotProvidedRequiresInsufficientContext(t *testing.T) {
	p := &portfolio.Context{State: portfolio.StateNotProvided}

	t.Run("rejects definitive assessment", func(t *testing.T) {
		ctx := &Context{
			Symbol:     "600519.SH",
			Assessment: AssessmentCaution,
			Summary:    "cannot know portfolio risk",
			Confidence: 0.4,
		}
		err := Validate(ctx, validIntent(), p)
		if err == nil || !strings.Contains(err.Error(), "INSUFFICIENT_CONTEXT") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("accepts insufficient context", func(t *testing.T) {
		ctx := &Context{
			Symbol:     "600519.SH",
			Assessment: AssessmentInsufficientContext,
			Summary:    "portfolio was not provided",
			Unresolved: []Item{
				{
					Claim: "portfolio exposure cannot be assessed",
					References: []Reference{
						{Domain: ReferenceTradeIntent},
					},
				},
			},
			Confidence: 0.3,
		}
		if err := Validate(ctx, validIntent(), p); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
}

func TestValidateRejectsInventedPortfolioReference(t *testing.T) {
	ctx := &Context{
		Symbol:     "600519.SH",
		Assessment: AssessmentHighRisk,
		Summary:    "invalid invented position reference",
		Findings: []Item{
			{
				Claim: "invented position",
				References: []Reference{
					{Domain: ReferencePortfolio, Symbol: "300750.SZ"},
				},
			},
		},
		Confidence: 0.8,
	}
	err := Validate(ctx, validIntent(), positionsPortfolio())
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsPortfolioReferenceWhenNotProvided(t *testing.T) {
	ctx := &Context{
		Symbol:     "600519.SH",
		Assessment: AssessmentInsufficientContext,
		Summary:    "portfolio unavailable",
		Unresolved: []Item{
			{
				Claim: "cannot inspect position",
				References: []Reference{
					{Domain: ReferencePortfolio},
				},
			},
		},
		Confidence: 0.2,
	}
	err := Validate(
		ctx,
		validIntent(),
		&portfolio.Context{State: portfolio.StateNotProvided},
	)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsSymbolMismatch(t *testing.T) {
	ctx := &Context{
		Symbol:     "000001.SZ",
		Assessment: AssessmentAcceptable,
		Summary:    "test",
		Confidence: 0.5,
	}
	err := Validate(ctx, validIntent(), emptyPortfolio())
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidReferenceDomain(t *testing.T) {
	ctx := &Context{
		Symbol:     "600519.SH",
		Assessment: AssessmentCaution,
		Summary:    "test",
		Findings: []Item{
			{
				Claim:      "bad ref",
				References: []Reference{{Domain: "EXTERNAL"}},
			},
		},
		Confidence: 0.5,
	}
	err := Validate(ctx, validIntent(), emptyPortfolio())
	if err == nil || !strings.Contains(err.Error(), "invalid reference domain") {
		t.Fatalf("error = %v", err)
	}
}
