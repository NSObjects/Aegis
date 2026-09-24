package technical

import (
	"errors"
	"fmt"
	"strings"
)

func Validate(v *Context) error {
	if v == nil {
		return errors.New("technical context is nil")
	}

	if strings.TrimSpace(v.Symbol) == "" {
		return errors.New("symbol is required")
	}

	if strings.TrimSpace(v.RequestedAsOf) == "" {
		return errors.New("requested_as_of is required")
	}

	switch v.Status {
	case StatusNormal:
		if strings.TrimSpace(v.EffectiveAsOf) == "" {
			return errors.New(
				"effective_as_of is required for NORMAL status",
			)
		}

		if v.Snapshot == nil {
			return errors.New(
				"snapshot is required for NORMAL status",
			)
		}

		if len(v.Evidence) == 0 {
			return errors.New(
				"evidence is required for NORMAL status",
			)
		}

	case StatusSuspended:
		if strings.TrimSpace(v.LastTradeDate) == "" {
			return errors.New(
				"last_trade_date is required for SUSPENDED status",
			)
		}

		if len(v.Risk) == 0 {
			return errors.New(
				"risk is required for SUSPENDED status",
			)
		}

	case StatusNotListed, StatusDelisted, StatusNoData:
		if len(v.Risk) == 0 {
			return fmt.Errorf(
				"risk is required for %s status",
				v.Status,
			)

	default:
		return fmt.Errorf(
			"invalid technical status: %q",
			v.Status,
		)
	}

	if v.Confidence < 0 || v.Confidence > 1 {
		return fmt.Errorf(
			"confidence out of range: %f",
			v.Confidence,
		)
	}

	return nil
}
