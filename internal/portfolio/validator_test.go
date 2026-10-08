package portfolio

import (
	"math"
	"strings"
	"testing"
)

func TestValidateNotProvided(t *testing.T) {
	if err := Validate(&Context{State: StateNotProvided}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateEmptyPortfolio(t *testing.T) {
	ctx := &Context{
		State: StateEmpty,
		AsOf:  "2026-10-07T15:00:00+08:00",
	}
	if err := Validate(ctx); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidatePortfolioWithPositions(t *testing.T) {
	ctx := &Context{
		State: StatePositions,
		AsOf:  "2026-10-07T15:00:00+08:00",
		Positions: []Position{
			{Symbol: "600519.SH", Quantity: 100},
			{Symbol: "000001.SZ", Quantity: 200},
		},
	}
	if err := Validate(ctx); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidatePreservesNotProvidedVsEmpty(t *testing.T) {
	t.Run("not provided rejects observed timestamp", func(t *testing.T) {
		err := Validate(&Context{
			State: StateNotProvided,
			AsOf:  "2026-10-07T15:00:00+08:00",
		})
		if err == nil || !strings.Contains(err.Error(), "must be empty") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("empty requires observed timestamp", func(t *testing.T) {
		err := Validate(&Context{State: StateEmpty})
		if err == nil || !strings.Contains(err.Error(), "as_of is required") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestValidateRejectsInconsistentPositionsState(t *testing.T) {
	tests := []struct {
		name string
		ctx  *Context
		want string
	}{
		{
			name: "positions without holdings",
			ctx: &Context{
				State: StatePositions,
				AsOf:  "2026-10-07T15:00:00+08:00",
			},
			want: "positions are required",
		},
		{
			name: "empty with holdings",
			ctx: &Context{
				State: StateEmpty,
				AsOf:  "2026-10-07T15:00:00+08:00",
				Positions: []Position{{Symbol: "600519.SH", Quantity: 100}},
			},
			want: "positions must be empty",
		},
		{
			name: "not provided with holdings",
			ctx: &Context{
				State: StateNotProvided,
				Positions: []Position{{Symbol: "600519.SH", Quantity: 100}},
			},
			want: "positions must be empty",
		},
		{
			name: "invalid timestamp",
			ctx: &Context{
				State: StateEmpty,
				AsOf:  "20261007",
			},
			want: "RFC3339",
		},
		{
			name: "nonpositive quantity",
			ctx: &Context{
				State: StatePositions,
				AsOf:  "2026-10-07T15:00:00+08:00",
				Positions: []Position{{Symbol: "600519.SH", Quantity: 0}},
			},
			want: "quantity must be positive",
		},
		{
			name: "duplicate symbol",
			ctx: &Context{
				State: StatePositions,
				AsOf:  "2026-10-07T15:00:00+08:00",
				Positions: []Position{
					{Symbol: "600519.SH", Quantity: 100},
					{Symbol: "600519.SH", Quantity: 200},
				},
			},
			want: "duplicated",
		},
		{
			name: "lowercase symbol",
			ctx: &Context{
				State: StatePositions,
				AsOf:  "2026-10-07T15:00:00+08:00",
				Positions: []Position{{Symbol: "600519.sh", Quantity: 100}},
			},
			want: "must be uppercase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.ctx)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want contains %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsNonfinitePositionQuantities(t *testing.T) {
	for _, tc := range []struct {
		name string
		quantity float64
	}{
		{name: "NaN", quantity: math.NaN()},
		{name: "positive infinity", quantity: math.Inf(1)},
		{name: "negative infinity", quantity: math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(&Context{
				State: StatePositions,
				AsOf: "2026-10-08T10:00:00+08:00",
				Positions: []Position{{Symbol:"600519.SH", Quantity:tc.quantity}},
			})
			if err == nil || !strings.Contains(err.Error(),"quantity must be positive") {
				t.Fatalf("error = %v",err)
			}
		})
	}
}
