package tradeintent

import (
	"Aegis/internal/decision"
	"fmt"
)

func FromDecision(d *decision.Decision) (*Intent, error) {
	if err := decision.Validate(d); err != nil {
		return nil, fmt.Errorf("invalid trading decision: %w", err)
	}

	var status Status
	switch d.Action {
	case decision.ActionWatch:
		status = StatusMonitor
	case decision.ActionAvoid:
		status = StatusRejected
	case decision.ActionCandidate:
		status = StatusCandidate
	default:
		return nil, fmt.Errorf("unsupported trading action: %q", d.Action)
	}

	intent := &Intent{
		Symbol:     d.Symbol,
		Thesis:     d.MarketView,
		Evidence:   append([]string(nil), d.Evidence...),
		Risk:       append([]string(nil), d.Risk...),
		Status:     status,
		Confidence: d.Confidence,
	}
	if err := Validate(intent); err != nil {
		return nil, err
	}
	return intent, nil
}
