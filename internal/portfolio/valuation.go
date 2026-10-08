package portfolio

import (
	"errors"
	"fmt"
	"math"
	"math/big"
)

// Monetary observations use integer fen, never floating-point CNY.
// The scoped total is ONLY cash plus these explicitly valued positions,
// with no loans, other assets, liabilities, pending orders or hidden balances.
const (
	ValuationScopeCashAndPositions = "CASH_AND_POSITIONS"
	CurrencyCNY                    = "CNY"
)

type PositionValue struct {
	Symbol         string `json:"symbol"`
	MarketValueFen int64  `json:"market_value_fen"`
}

type Valuation struct {
	Scope         string          `json:"scope"`
	Currency      string          `json:"currency"`
	TotalValueFen int64           `json:"total_value_fen"`
	CashFen       int64           `json:"cash_fen"`
	PositionValues []PositionValue `json:"position_values"`
}

type PositionExposure struct {
	Symbol         string `json:"symbol"`
	MarketValueFen int64  `json:"market_value_fen"`
	WeightBps      int64  `json:"weight_bps"`
}

// Exposure is a deterministic observation of CURRENT weights, not a projected
// trade outcome. WeightBps is truncated down, so weights may not sum to 10000.
// When total value is zero, CashWeightBps is nil (undefined, not zero).
type Exposure struct {
	AsOf               string             `json:"as_of"`
	Scope              string             `json:"scope"`
	Currency           string             `json:"currency"`
	TotalValueFen      int64              `json:"total_value_fen"`
	CashFen            int64              `json:"cash_fen"`
	InvestedValueFen   int64              `json:"invested_value_fen"`
	CashWeightBps      *int64             `json:"cash_weight_bps,omitempty"`
	Positions          []PositionExposure `json:"positions"`
}

// validateValuation ensures every position is accounted for exactly once,
// cash and positions reconcile to the scoped total, and no invented assets
// are silently folded into the snapshot.
func validateValuation(book *Context) error {
	v := book.Valuation
	if v == nil {
		return nil
	}
	if book.State == StateNotProvided {
		return errors.New("valuation is not allowed when portfolio is NOT_PROVIDED")
	}
	if v.Scope != ValuationScopeCashAndPositions {
		return fmt.Errorf("valuation scope %q must be %q", v.Scope, ValuationScopeCashAndPositions)
	}
	if v.Currency != CurrencyCNY {
		return fmt.Errorf("valuation currency %q must be %q", v.Currency, CurrencyCNY)
	}
	if v.TotalValueFen < 0 || v.CashFen < 0 {
		return errors.New("valuation total_value_fen and cash_fen must be nonnegative")
	}

	positions := make(map[string]struct{}, len(book.Positions))
	for _, pos := range book.Positions {
		positions[pos.Symbol] = struct{}{}
	}
	seen := make(map[string]struct{}, len(v.PositionValues))
	sum := v.CashFen
	for i, value := range v.PositionValues {
		if _, ok := positions[value.Symbol]; !ok {
			return fmt.Errorf("valuation position %d symbol %q is not in observed portfolio", i, value.Symbol)
		}
		if _, duplicate := seen[value.Symbol]; duplicate {
			return fmt.Errorf("valuation symbol %q is duplicated", value.Symbol)
		}
		seen[value.Symbol] = struct{}{}
		if value.MarketValueFen < 0 {
			return fmt.Errorf("valuation position %q market_value_fen must be nonnegative", value.Symbol)
		}
		if value.MarketValueFen > math.MaxInt64-sum {
			return errors.New("valuation cash plus positions overflows int64")
		}
		sum += value.MarketValueFen
	}
	if len(seen) != len(positions) {
		return fmt.Errorf("valuation must provide market value for every observed position (got %d, want %d)",
			len(seen), len(positions))
	}
	if sum != v.TotalValueFen {
		return fmt.Errorf("valuation does not reconcile: cash plus positions = %d fen, total_value_fen = %d",
			sum, v.TotalValueFen)
	}
	if book.State == StatePositions && v.TotalValueFen == 0 {
		return errors.New("valued POSITIONS portfolio requires positive total_value_fen")
	}
	return nil
}

// ComputeExposure derives current portfolio weights strictly from the
// caller-observed and reconciled valuation. Nil means no valuation was given;
// callers MUST NOT interpret nil as a zero-exposure portfolio.
func ComputeExposure(book *Context) (*Exposure, error) {
	if err := Validate(book); err != nil {
		return nil, fmt.Errorf("invalid portfolio for exposure: %w", err)
	}
	if book.Valuation == nil {
		return nil, nil
	}
	v := book.Valuation
	result := &Exposure{
		AsOf: book.AsOf, Scope: v.Scope, Currency: v.Currency,
		TotalValueFen: v.TotalValueFen, CashFen: v.CashFen,
		InvestedValueFen: v.TotalValueFen - v.CashFen,
		Positions: make([]PositionExposure, 0, len(v.PositionValues)),
	}
	if v.TotalValueFen > 0 {
		weight := weightBps(v.CashFen, v.TotalValueFen)
		result.CashWeightBps = &weight
	}
	valuations := make(map[string]int64, len(v.PositionValues))
	for _, value := range v.PositionValues {
		valuations[value.Symbol] = value.MarketValueFen
	}
	// Preserve the original portfolio position order so downstream evidence
	// can be matched to the validated caller snapshot.
	for _, position := range book.Positions {
		value := valuations[position.Symbol]
		result.Positions = append(result.Positions, PositionExposure{
			Symbol: position.Symbol,
			MarketValueFen: value,
			WeightBps: weightBps(value, v.TotalValueFen),
		})
	}
	return result, nil
}

func weightBps(value, total int64) int64 {
	if total <= 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(value), big.NewInt(10000))
	return new(big.Int).Quo(numerator, big.NewInt(total)).Int64()
}
