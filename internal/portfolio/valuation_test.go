package portfolio

import (
	"math"
	"strings"
	"testing"
)

func observedBook() *Context {
	return &Context{
		State: StatePositions,
		AsOf: "2026-10-08T09:00:00+08:00",
		Positions: []Position{
			{Symbol: "600519.SH", Quantity: 100},
			{Symbol: "000001.SZ", Quantity: 200},
		},
		Valuation: &Valuation{
			Scope: ValuationScopeCashAndPositions,
			Currency: CurrencyCNY,
			TotalValueFen: 100000,
			CashFen: 30000,
			PositionValues: []PositionValue{
				{Symbol: "000001.SZ", MarketValueFen: 10000},
				{Symbol: "600519.SH", MarketValueFen: 60000},
			},
		},
	}
}

func TestObservedValuationComputesAuditableExposure(t *testing.T) {
	book := observedBook()
	got, err := ComputeExposure(book)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.TotalValueFen != 100000 || got.CashFen != 30000 ||
		got.InvestedValueFen != 70000 || got.CashWeightBps == nil ||
		*got.CashWeightBps != 3000 {
		t.Fatalf("incorrect exposure totals: %+v", got)
	}
	if len(got.Positions) != 2 ||
		got.Positions[0].Symbol != "600519.SH" ||
		got.Positions[0].WeightBps != 6000 ||
		got.Positions[1].Symbol != "000001.SZ" ||
		got.Positions[1].WeightBps != 1000 {
		t.Fatalf("positions must follow observed portfolio and correct weights: %+v", got.Positions)
	}
	if got.AsOf != book.AsOf || got.Currency != CurrencyCNY {
		t.Fatalf("snapshot identity lost: %+v", got)
	}
}

func TestObservedValuationDoesNotInferUnknownExposure(t *testing.T) {
	for _, book := range []*Context{
		{State: StateNotProvided},
		{State: StateEmpty, AsOf: "2026-10-08T09:00:00+08:00"},
		{State: StatePositions, AsOf: "2026-10-08T09:00:00+08:00",
			Positions: []Position{{Symbol:"600519.SH",Quantity:100}}},
	} {
		got, err := ComputeExposure(book)
		if err != nil || got != nil {
			t.Fatalf("quantity or state alone must not infer value: %+v, err=%v", got, err)
		}
	}
}

func TestEmptyZeroValueValuationHasUndefinedWeights(t *testing.T) {
	book := &Context{
		State: StateEmpty,
		AsOf: "2026-10-08T09:00:00+08:00",
		Valuation: &Valuation{
			Scope: ValuationScopeCashAndPositions,
			Currency: CurrencyCNY,
			TotalValueFen: 0, CashFen: 0,
		},
	}
	got, err := ComputeExposure(book)
	if err != nil || got == nil || got.CashWeightBps != nil ||
		len(got.Positions) != 0 || got.InvestedValueFen != 0 {
		t.Fatalf("zero total cannot produce a percentage: %+v, err=%v", got, err)
	}
}

func TestPortfolioValuationValidationRejectsInvalidFinancialFacts(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Context)
		want string
	}{
		{"not provided with valuation", func(b *Context) { b.State = StateNotProvided; b.Positions = nil; b.AsOf = "" }, "NOT_PROVIDED"},
		{"unknown valuation scope", func(b *Context) { b.Valuation.Scope = "ALL_ASSETS" }, "scope"},
		{"currency mismatch", func(b *Context) { b.Valuation.Currency = "USD" }, "currency"},
		{"negative cash", func(b *Context) { b.Valuation.CashFen = -1 }, "nonnegative"},
		{"negative total", func(b *Context) { b.Valuation.TotalValueFen = -1 }, "nonnegative"},
		{"negative position value", func(b *Context) { b.Valuation.PositionValues[0].MarketValueFen = -1 }, "nonnegative"},
		{"total mismatch", func(b *Context) { b.Valuation.TotalValueFen++ }, "does not reconcile"},
		{"missing value", func(b *Context) { b.Valuation.PositionValues = b.Valuation.PositionValues[:1] }, "every observed position"},
		{"duplicate value", func(b *Context) { b.Valuation.PositionValues[0].Symbol = "600519.SH" }, "duplicated"},
		{"invented value", func(b *Context) { b.Valuation.PositionValues[0].Symbol = "300750.SZ" }, "not in observed portfolio"},
		{"integer overflow", func(b *Context) { b.Valuation.CashFen = math.MaxInt64 }, "overflows"},
		{"zero-value holdings", func(b *Context) {
			b.Valuation.CashFen = 0; b.Valuation.TotalValueFen = 0
			for i := range b.Valuation.PositionValues {
				b.Valuation.PositionValues[i].MarketValueFen = 0
			}
		}, "positive total_value_fen"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			book := observedBook()
			tc.edit(book)
			if err := Validate(book); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate error=%v, want %q", err, tc.want)
			}
			if got, err := ComputeExposure(book); err == nil || got != nil {
				t.Fatalf("invalid valuation must never produce exposure: %+v error=%v", got, err)
			}
		})
	}
}

func TestCurrentExposureBasisPointsAreTruncatedWithoutFloatRounding(t *testing.T) {
	book := &Context{
		State: StatePositions, AsOf: "2026-10-08T09:00:00+08:00",
		Positions: []Position{{Symbol:"600519.SH",Quantity:1}},
		Valuation: &Valuation{
			Scope: ValuationScopeCashAndPositions, Currency: CurrencyCNY,
			TotalValueFen: 3, CashFen: 2,
			PositionValues: []PositionValue{{Symbol:"600519.SH",MarketValueFen:1}},
		},
	}
	got, err := ComputeExposure(book)
	if err!=nil {t.Fatal(err)}
	if got.Positions[0].WeightBps!=3333 || *got.CashWeightBps!=6666 {
		t.Fatalf("expected explicit downward truncation, got %+v",got)
	}
}
