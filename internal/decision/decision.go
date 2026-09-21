package decision

import "fmt"

type Action string

const (
	ActionWatch     Action = "WATCH"
	ActionAvoid     Action = "AVOID"
	ActionCandidate Action = "CANDIDATE"
)

type Decision struct {
	Symbol     string   `json:"symbol"`
	MarketView string   `json:"market_view"`
	Evidence   []string `json:"evidence"`
	Risk       []string `json:"risk"`
	Action     Action   `json:"action"`
	Confidence float64  `json:"confidence"`
}

func Validate(d *Decision) error {
	if d.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}

	if d.MarketView == "" {
		return fmt.Errorf("market view is required")
	}

	switch d.Action {
	case ActionWatch, ActionAvoid, ActionCandidate:
	default:
		return fmt.Errorf("invalid action: %s", d.Action)
	}

	if d.Confidence < 0 || d.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}

	if len(d.Evidence) == 0 {
		return fmt.Errorf("evidence is required")
	}

	return nil
}
