package portfolio

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Limits are caller-authored constraints, NOT facts inferred from account
// holdings and NOT a broker's authorization to place trades. They are kept
// separate from the observed Context and optional monetary Valuation.
type LimitKind string

const (
	LimitMaxSinglePositionWeight LimitKind = "MAX_SINGLE_POSITION_WEIGHT"
	LimitMaxInvestedWeight       LimitKind = "MAX_INVESTED_WEIGHT"
)

type LimitRule struct {
	Kind     LimitKind `json:"kind"`
	LimitBps *int64    `json:"limit_bps"`
}

type Limits struct {
	PolicyID string      `json:"policy_id"`
	AsOf     string      `json:"as_of"`
	Scope    string      `json:"scope"`
	Currency string      `json:"currency"`
	Rules    []LimitRule `json:"rules"`
}

type LimitReviewStatus string

const (
	LimitReviewPass             LimitReviewStatus = "PASS"
	LimitReviewBreach           LimitReviewStatus = "BREACH"
	LimitReviewInsufficientData LimitReviewStatus = "INSUFFICIENT_DATA"
)

type LimitCheck struct {
	Kind              LimitKind         `json:"kind"`
	Symbol            string            `json:"symbol,omitempty"`
	LimitBps          int64             `json:"limit_bps"`
	ObservedWeightBps *int64            `json:"observed_weight_bps,omitempty"`
	Status            LimitReviewStatus `json:"status"`
}

// LimitReview checks CURRENT observed holdings only. It is not a projected
// post-trade risk judgment, buy-power calculation or execution approval.
type LimitReview struct {
	PolicyID   string            `json:"policy_id"`
	PolicyAsOf string            `json:"policy_as_of"`
	AccountAsOf string           `json:"account_as_of,omitempty"`
	Status     LimitReviewStatus `json:"status"`
	Checks     []LimitCheck      `json:"checks"`
}

func ValidateLimits(limits *Limits) error {
	if limits == nil {
		return errors.New("risk limits are nil")
	}
	if strings.TrimSpace(limits.PolicyID) == "" {
		return errors.New("risk policy_id is required")
	}
	if _, err := time.Parse(time.RFC3339, limits.AsOf); err != nil {
		return fmt.Errorf("risk limits as_of must be RFC3339: %w", err)
	}
	if limits.Scope != ValuationScopeCashAndPositions {
		return fmt.Errorf("risk limits scope %q must be %q", limits.Scope, ValuationScopeCashAndPositions)
	}
	if limits.Currency != CurrencyCNY {
		return fmt.Errorf("risk limits currency %q must be %q", limits.Currency, CurrencyCNY)
	}
	if len(limits.Rules) == 0 {
		return errors.New("risk limits must include at least one rule")
	}
	seen := make(map[LimitKind]struct{}, len(limits.Rules))
	for i, rule := range limits.Rules {
		switch rule.Kind {
		case LimitMaxSinglePositionWeight, LimitMaxInvestedWeight:
		default:
			return fmt.Errorf("risk rule %d has unsupported kind %q", i, rule.Kind)
		}
		if _, exists := seen[rule.Kind]; exists {
			return fmt.Errorf("risk rule %d kind %q is duplicated", i, rule.Kind)
		}
		seen[rule.Kind] = struct{}{}
		if rule.LimitBps == nil {
			return fmt.Errorf("risk rule %d limit_bps must be explicitly provided", i)
		}
		if *rule.LimitBps < 0 || *rule.LimitBps > 10000 {
			return fmt.Errorf("risk rule %d limit_bps must be between 0 and 10000", i)
		}
	}
	return nil
}

func ValidateLimitsAt(limits *Limits, requestedAsOf time.Time) error {
	if err := ValidateLimits(limits); err != nil {
		return err
	}
	if requestedAsOf.IsZero() {
		return errors.New("research requested_as_of is required for risk limits")
	}
	policyAsOf, _ := time.Parse(time.RFC3339, limits.AsOf)
	if policyAsOf.After(requestedAsOf) {
		return fmt.Errorf("risk policy as_of %q is after research requested_as_of %q",
			limits.AsOf, requestedAsOf.Format(time.RFC3339))
	}
	return nil
}

// EvaluateCurrentLimits compares exact integer-fen fractions. The displayed
// weight in basis points is truncated, but the breach test is NOT rounded:
// for example, 3000.1 bps breaches a 3000 bps limit.
func EvaluateCurrentLimits(book *Context, limits *Limits, requestedAsOf time.Time) (*LimitReview, error) {
	if limits == nil {
		return nil, nil
	}
	if err := ValidateLimitsAt(limits, requestedAsOf); err != nil {
		return nil, fmt.Errorf("invalid risk limits: %w", err)
	}
	if err := ValidateAt(book, requestedAsOf, 0); err != nil {
		return nil, fmt.Errorf("invalid portfolio snapshot: %w", err)
	}
	exposure, err := ComputeExposure(book)
	if err != nil {
		return nil, fmt.Errorf("compute current exposure: %w", err)
	}
	result := &LimitReview{
		PolicyID: limits.PolicyID,
		PolicyAsOf: limits.AsOf,
		Status: LimitReviewPass,
		Checks: make([]LimitCheck, 0, len(limits.Rules)),
	}
	if exposure != nil {
		result.AccountAsOf = exposure.AsOf
	}
	for _, rule := range limits.Rules {
		check := LimitCheck{Kind: rule.Kind, LimitBps: *rule.LimitBps}
		if exposure == nil || exposure.TotalValueFen == 0 {
			check.Status = LimitReviewInsufficientData
		} else {
			var amount int64
			switch rule.Kind {
			case LimitMaxSinglePositionWeight:
				for _, pos := range exposure.Positions {
					if pos.MarketValueFen > amount {
						amount = pos.MarketValueFen
						check.Symbol = pos.Symbol
					}
				}
			case LimitMaxInvestedWeight:
				amount = exposure.InvestedValueFen
			}
			weight := weightBps(amount, exposure.TotalValueFen)
			check.ObservedWeightBps = &weight
			if exceedsLimit(amount, exposure.TotalValueFen, *rule.LimitBps) {
				check.Status = LimitReviewBreach
			} else {
				check.Status = LimitReviewPass
			}
		}
		result.Checks = append(result.Checks, check)
		if check.Status == LimitReviewBreach {
			result.Status = LimitReviewBreach
		} else if check.Status == LimitReviewInsufficientData && result.Status != LimitReviewBreach {
			result.Status = LimitReviewInsufficientData
		}
	}
	return result, nil
}

func exceedsLimit(valueFen, totalFen, limitBps int64) bool {
	value := new(big.Int).Mul(big.NewInt(valueFen), big.NewInt(10000))
	limit := new(big.Int).Mul(big.NewInt(totalFen), big.NewInt(limitBps))
	return value.Cmp(limit) > 0
}
