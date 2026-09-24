package event

import (
	"errors"
	"fmt"
	"strings"
)

func Validate(v *Context) error {
	if v == nil {
		return errors.New("event context is nil")
	}

	if strings.TrimSpace(v.Symbol) == "" {
		return errors.New("symbol is required")
	}
	if strings.TrimSpace(v.RequestedAsOf) == "" {
		return errors.New("requested_as_of is required")
	}
	if strings.TrimSpace(v.WindowStart) == "" {
		return errors.New("window_start is required")
	}
	if strings.TrimSpace(v.WindowEnd) == "" {
		return errors.New("window_end is required")
	}

	if !v.Coverage.Announcements &&
		!v.Coverage.News &&
		!v.Coverage.Policy {
		return errors.New("at least one event source must be covered")
	}

	switch v.Status {
	case StatusNormal:
		if len(v.Events) == 0 {
			return errors.New("events are required for NORMAL status")
		}
		if len(v.Evidence) == 0 {
			return errors.New("evidence is required for NORMAL status")
		}
	case StatusNoEvent:
		if len(v.Events) != 0 {
			return errors.New("events must be empty for NO_EVENT status")
		}
		if len(v.Evidence) == 0 {
			return errors.New("evidence is required for NO_EVENT status")
		}
	case StatusNoData:
		if len(v.Risk) == 0 {
			return errors.New("risk is required for NO_DATA status")
		}
	default:
		return fmt.Errorf("invalid event status: %q", v.Status)
	}

	for i := range v.Events {
		if err := validateItem(&v.Events[i]); err != nil {
			return fmt.Errorf("event[%d]: %w", i, err)
		}
	}

	if v.Confidence < 0 || v.Confidence > 1 {
		return fmt.Errorf(
			"confidence out of range: %f",
			v.Confidence,
		)
	}

	return nil
}

func validateItem(v *Item) error {
	if v == nil {
		return errors.New("event item is nil")
	}

	switch v.Type {
	case TypeAnnouncement, TypeNews, TypePolicy, TypeRegulatory, TypeOther:
	default:
		return fmt.Errorf("invalid type: %q", v.Type)
	}

	if strings.TrimSpace(v.PublishedAt) == "" {
		return errors.New("published_at is required")
	}
	if strings.TrimSpace(v.Source) == "" {
		return errors.New("source is required")
	}
	if strings.TrimSpace(v.Title) == "" {
		return errors.New("title is required")
	}
	if strings.TrimSpace(v.Fact) == "" {
		return errors.New("fact is required")
	}
	if strings.TrimSpace(v.Interpretation) == "" {
		return errors.New("interpretation is required")
	}

	switch v.Impact {
	case ImpactPositive, ImpactNegative, ImpactNeutral, ImpactMixed, ImpactUnknown:
	default:
		return fmt.Errorf("invalid impact: %q", v.Impact)
	}

	switch v.Materiality {
	case MaterialityLow, MaterialityMedium, MaterialityHigh:
	default:
		return fmt.Errorf("invalid materiality: %q", v.Materiality)
	}

	return nil
}
