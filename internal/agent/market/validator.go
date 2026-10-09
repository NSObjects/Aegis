package market

import (
	"errors"
	"fmt"
	"math"
)

func Validate(v *Context) error {
	if v == nil {
		return errors.New("market context is nil")
	}

	if v.AsOf == "" {
		return errors.New("as_of is required")
	}

	switch v.Regime {
	case RegimeRiskOn, RegimeMixed, RegimeRiskOff:
	default:
		return fmt.Errorf("invalid regime: %s", v.Regime)
	}

	if math.IsNaN(v.Confidence) || v.Confidence < 0 || v.Confidence > 1 {
		return fmt.Errorf(
			"confidence out of range: %f",
			v.Confidence,
		)
	}

	if len(v.Evidence) == 0 {
		return errors.New("evidence is empty")
	}

	return nil
}
