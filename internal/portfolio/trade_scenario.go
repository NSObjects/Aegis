package portfolio

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// TradeScenario is caller-authored, hypothetical CNY notional movement.
// It is NOT an order: price, share quantity, fees, settlement, fills and
// broker order workflows are outside the product scope.
type ScenarioSide string

const (
	ScenarioBuy  ScenarioSide = "BUY"
	ScenarioSell ScenarioSide = "SELL"
)

type TradeScenario struct {
	ScenarioID string       `json:"scenario_id"`
	AsOf       string       `json:"as_of"`
	Symbol     string       `json:"symbol"`
	Side       ScenarioSide `json:"side"`
	Currency   string       `json:"currency"`
	NotionalFen *int64      `json:"notional_fen"`
}

type ScenarioStatus string

const (
	ScenarioWithinLimits       ScenarioStatus = "WITHIN_MODELED_LIMITS"
	ScenarioBreach             ScenarioStatus = "BREACH"
	ScenarioInsufficientData   ScenarioStatus = "INSUFFICIENT_DATA"
	ScenarioInsufficientCash   ScenarioStatus = "INSUFFICIENT_CASH"
	ScenarioInsufficientValue  ScenarioStatus = "INSUFFICIENT_POSITION_VALUE"
	ScenarioNotApplicable      ScenarioStatus = "NOT_APPLICABLE"
)

type ScenarioLimitCheck struct {
	Kind               LimitKind         `json:"kind"`
	Symbol             string            `json:"symbol,omitempty"`
	LimitBps           int64             `json:"limit_bps"`
	ProjectedWeightBps int64             `json:"projected_weight_bps"`
	Status             LimitReviewStatus `json:"status"`
}

// ScenarioReview is an indicative mathematical what-if, always a research-only diagnostic.
// All projected amounts describe a constant-NAV, zero-fee hypothetical cash
// / position-value transfer; no prices or executable quantities are implied.
type ScenarioReview struct {
	ScenarioID          string               `json:"scenario_id"`
	ScenarioAsOf        string               `json:"scenario_as_of"`
	AccountAsOf         string               `json:"account_as_of,omitempty"`
	PolicyID            string               `json:"policy_id"`
	Symbol              string               `json:"symbol"`
	Side                ScenarioSide         `json:"side"`
	NotionalFen         int64                `json:"notional_fen"`
	Status              ScenarioStatus       `json:"status"`
	Reason              string               `json:"reason"`
	ProjectedCashFen    *int64               `json:"projected_cash_fen,omitempty"`
	ProjectedInvestedFen *int64              `json:"projected_invested_fen,omitempty"`
	Checks              []ScenarioLimitCheck `json:"checks,omitempty"`
}

func ValidateTradeScenarioAt(
	scenario *TradeScenario,
	book *Context,
	limits *Limits,
	requestedAsOf time.Time,
	expectedSymbol string,
) error {
	if scenario == nil {
		return nil
	}
	if err := ValidateLimitsAt(limits, requestedAsOf); err != nil {
		return fmt.Errorf("risk policy required for trade scenario: %w", err)
	}
	if err := ValidateAt(book, requestedAsOf, 0); err != nil {
		return fmt.Errorf("invalid account for trade scenario: %w", err)
	}
	if strings.TrimSpace(scenario.ScenarioID) == "" {
		return errors.New("scenario_id is required")
	}
	at, err := time.Parse(time.RFC3339, scenario.AsOf)
	if err != nil {
		return fmt.Errorf("trade scenario as_of must be RFC3339: %w", err)
	}
	if at.After(requestedAsOf) {
		return errors.New("trade scenario as_of is after research requested_as_of")
	}
	policyAt, _ := time.Parse(time.RFC3339, limits.AsOf)
	if at.Before(policyAt) {
		return errors.New("trade scenario as_of precedes risk policy as_of")
	}
	if book.State != StateNotProvided {
		bookAt, _ := time.Parse(time.RFC3339, book.AsOf)
		if at.Before(bookAt) {
			return errors.New("trade scenario as_of precedes observed account as_of")
		}
	}
	if strings.TrimSpace(scenario.Symbol) == "" ||
		scenario.Symbol != strings.TrimSpace(expectedSymbol) ||
		scenario.Symbol != strings.ToUpper(scenario.Symbol) {
		return fmt.Errorf("trade scenario symbol %q does not match research symbol %q", scenario.Symbol, expectedSymbol)
	}
	if scenario.Side != ScenarioBuy && scenario.Side != ScenarioSell {
		return fmt.Errorf("unsupported trade scenario side %q", scenario.Side)
	}
	if scenario.Currency != CurrencyCNY || scenario.Currency != limits.Currency {
		return fmt.Errorf("trade scenario currency must be %s", CurrencyCNY)
	}
	if scenario.NotionalFen == nil || *scenario.NotionalFen <= 0 {
		return errors.New("trade scenario notional_fen must be explicitly provided and positive")
	}
	return nil
}

// EvaluateTradeScenario never mutates an observed Context and does not invoke
// models, quote providers or trading services. The caller must separately
// enforce that the associated research TradeIntent is CANDIDATE.
func EvaluateTradeScenario(
	book *Context,
	limits *Limits,
	scenario *TradeScenario,
	requestedAsOf time.Time,
	expectedSymbol string,
) (*ScenarioReview, error) {
	if scenario == nil {
		return nil, nil
	}
	if err := ValidateTradeScenarioAt(scenario, book, limits, requestedAsOf, expectedSymbol); err != nil {
		return nil, err
	}
	out := &ScenarioReview{
		ScenarioID:scenario.ScenarioID, ScenarioAsOf:scenario.AsOf,
		PolicyID:limits.PolicyID, Symbol:scenario.Symbol,
		Side:scenario.Side, NotionalFen:*scenario.NotionalFen,
	}
	if book.State != StateNotProvided {
		out.AccountAsOf = book.AsOf
	}
	if book.Valuation == nil || book.Valuation.TotalValueFen == 0 {
		out.Status = ScenarioInsufficientData
		out.Reason = "必须提供经过核对的真实账户正估值，否则无法计算交易情景。"
		return out, nil
	}
	v := book.Valuation
	amount := *scenario.NotionalFen
	current := make(map[string]int64, len(v.PositionValues))
	for _, pos := range v.PositionValues {
		current[pos.Symbol] = pos.MarketValueFen
	}
	value, exists := current[scenario.Symbol]
	var cash int64
	switch scenario.Side {
	case ScenarioBuy:
		if amount > v.CashFen {
			out.Status = ScenarioInsufficientCash
			out.Reason = "假设买入金额超过已观测现金余额（尚未计入手续费）。"
			return out, nil
		}
		if value > math.MaxInt64-amount {
			return nil, errors.New("hypothetical position value overflows int64")
		}
		cash = v.CashFen-amount
		current[scenario.Symbol] = value+amount
	case ScenarioSell:
		if !exists || amount > value {
			out.Status = ScenarioInsufficientValue
			out.Reason = "假设卖出金额超过该标的已观测市值。"
			return out, nil
		}
		// Exact reconciliation guarantees total+market values can fit int64;
		// the converted cash remains at most the observed total.
		cash = v.CashFen+amount
		current[scenario.Symbol] = value-amount
	}
	invested := v.TotalValueFen-cash
	out.ProjectedCashFen = &cash
	out.ProjectedInvestedFen = &invested
	out.Status = ScenarioWithinLimits
	out.Reason = "仅执行当前风险限额的假设情景检查，属于研究性质，不是交易指令。"
	out.Checks = make([]ScenarioLimitCheck, 0, len(limits.Rules))
	for _, rule := range limits.Rules {
		check := ScenarioLimitCheck{Kind:rule.Kind, LimitBps:*rule.LimitBps}
		var projected int64
		switch rule.Kind {
		case LimitMaxSinglePositionWeight:
			// Sort-free selection: only the maximum amount is used; ties
			// choose lexicographically smaller symbols deterministically.
			for symbol, value := range current {
				if value > projected || (value == projected && value > 0 &&
					(check.Symbol == "" || symbol < check.Symbol)) {
					projected = value
					check.Symbol = symbol
				}
			}
		case LimitMaxInvestedWeight:
			projected = invested
		}
		check.ProjectedWeightBps = weightBps(projected, v.TotalValueFen)
		check.Status = LimitReviewPass
		if exceedsLimit(projected, v.TotalValueFen, *rule.LimitBps) {
			check.Status = LimitReviewBreach
			out.Status = ScenarioBreach
			out.Reason = "假设交易后的组合敞口超出了调用方设置的当前权重限制。"
		}
		out.Checks = append(out.Checks, check)
	}
	return out, nil
}

// BuildNotApplicableScenario records that research did NOT produce a
// candidate. No post-trade projection is calculated or implied.
func BuildNotApplicableScenario(s *TradeScenario, limits *Limits, reason string) *ScenarioReview {
	return &ScenarioReview{
		ScenarioID:s.ScenarioID, ScenarioAsOf:s.AsOf,
		PolicyID:limits.PolicyID, Symbol:s.Symbol, Side:s.Side,
		NotionalFen:*s.NotionalFen, Status:ScenarioNotApplicable,
		Reason:reason,
	}
}
