package tradeintent

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

func Validate(intent *Intent) error {
	if intent == nil {
		return errors.New("trade intent is nil")
	}
	if strings.TrimSpace(intent.Symbol) == "" {
		return errors.New("symbol is required")
	}
	if strings.TrimSpace(intent.Thesis) == "" {
		return errors.New("thesis is required")
	}
	switch intent.Status {
	case StatusMonitor, StatusRejected, StatusCandidate:
	default:
		return fmt.Errorf("invalid trade intent status: %q", intent.Status)
	}
	if math.IsNaN(intent.Confidence) || intent.Confidence < 0 || intent.Confidence > 1 {
		return errors.New("confidence must be between 0 and 1")
	}
	if len(intent.Evidence) == 0 {
		return errors.New("evidence is required")
	}
	for i, evidence := range intent.Evidence {
		if strings.TrimSpace(evidence) == "" {
			return fmt.Errorf("evidence %d is empty", i)
		}
	}
	for i, risk := range intent.Risk {
		if strings.TrimSpace(risk) == "" {
			return fmt.Errorf("risk %d is empty", i)
		}
	}
	return nil
}
