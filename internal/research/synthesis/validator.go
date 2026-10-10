package synthesis

import (
	"Aegis/internal/agent/event"
		"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

func ValidateInput(
	input Input,
) error {
	symbol := strings.TrimSpace(input.Symbol)
	if symbol == "" {
		return errors.New(
			"research synthesis symbol is required",
		)
	}

	if err := market.Validate(
		input.Market,
	); err != nil {
		return fmt.Errorf(
			"invalid market context: %w",
			err,
		)
	}
	if err := technical.Validate(
		input.Technical,
	); err != nil {
		return fmt.Errorf(
			"invalid technical context: %w",
			err,
		)
	}
	if err := event.Validate(
		input.Event,
	); err != nil {
		return fmt.Errorf(
			"invalid event context: %w",
			err,
		)
	}

	if input.Fundamental != nil {
		if input.Fundamental.Symbol != symbol || input.Fundamental.Income == nil || input.Fundamental.Income.Symbol != symbol || strings.TrimSpace(input.Fundamental.Interpretation)=="" {return fmt.Errorf("invalid fundamental context for %s",symbol)}
		if input.Fundamental.RequestedAsOf != input.Technical.RequestedAsOf {return fmt.Errorf("fundamental cutoff differs from technical requested date")}
		if input.Fundamental.Income.AnnouncementDate > input.Fundamental.RequestedAsOf {return fmt.Errorf("fundamental report announced after cutoff")}
	}
	if strings.TrimSpace(
		input.Technical.Symbol,
	) != symbol {
		return fmt.Errorf(
			"technical context symbol %q does not match synthesis symbol %q",
			input.Technical.Symbol,
			symbol,
		)
	}
	if strings.TrimSpace(
		input.Event.Symbol,
	) != symbol {
		return fmt.Errorf(
			"event context symbol %q does not match synthesis symbol %q",
			input.Event.Symbol,
			symbol,
		)
	}

	marketAsOf, err := time.Parse("20060102", input.Market.AsOf)
	if err != nil {
		return fmt.Errorf("invalid market as_of %q: %w", input.Market.AsOf, err)
	}
	technicalRequestedAsOf, err := time.Parse("20060102", input.Technical.RequestedAsOf)
	if err != nil {
		return fmt.Errorf("invalid technical requested_as_of %q: %w", input.Technical.RequestedAsOf, err)
	}
	if marketAsOf.After(technicalRequestedAsOf) {
		return fmt.Errorf(
			"market as_of %q is after technical requested_as_of %q",
			input.Market.AsOf,
			input.Technical.RequestedAsOf,
		)
	}

	return nil
}

func Validate(
	artifact *Artifact,
) error {
	if artifact == nil {
		return errors.New(
			"research artifact is nil",
		)
	}

	if err := ValidateInput(
		Input{
			Symbol:    artifact.Symbol,
			Market:    artifact.Market,
			Technical: artifact.Technical,
			Event:     artifact.Event,
			Fundamental: artifact.Fundamental,
		},
	); err != nil {
		return err
	}

	return validateSynthesis(
		artifact.Synthesis,
		artifact.Event,
		artifact.Fundamental != nil,
	)
}

func validateSynthesis(
	value Synthesis,
	eventContext *event.Context,
	fundamentalPresent ...bool,
) error {
	if strings.TrimSpace(value.Summary) == "" {
		return errors.New(
			"research synthesis summary is required",
		)
	}
	if math.IsNaN(value.Confidence) ||
		value.Confidence < 0 ||
		value.Confidence > 1 {
		return fmt.Errorf(
			"research synthesis confidence out of range: %f",
			value.Confidence,
		)
	}

	available := len(fundamentalPresent)>0 && fundamentalPresent[0]
	eventIDs := make(
		map[string]struct{},
		len(eventContext.Events),
	)
	for _, item := range eventContext.Events {
		eventIDs[item.ID] = struct{}{}
	}

	for index, agreement := range value.Agreements {
		if strings.TrimSpace(
			agreement.Claim,
		) == "" {
			return fmt.Errorf(
				"agreement %d claim is required",
				index,
			)
		}
		if len(agreement.References) == 0 {
			return fmt.Errorf(
				"agreement %d requires at least one reference",
				index,
			)
		}
		for referenceIndex, reference :=
			range agreement.References {
			if err := validateReference(
				reference,
				eventIDs,
				available,
			); err != nil {
				return fmt.Errorf(
					"agreement %d reference %d: %w",
					index,
					referenceIndex,
					err,
				)
			}
		}
	}

	for index, conflict := range value.Conflicts {
		if strings.TrimSpace(
			conflict.Description,
		) == "" {
			return fmt.Errorf(
				"conflict %d description is required",
				index,
			)
		}
		if len(conflict.References) < 2 {
			return fmt.Errorf(
				"conflict %d requires at least two references",
				index,
			)
		}

		domains := make(map[Domain]struct{})
		for referenceIndex, reference :=
			range conflict.References {
			if err := validateReference(
				reference,
				eventIDs,
				available,
			); err != nil {
				return fmt.Errorf(
					"conflict %d reference %d: %w",
					index,
					referenceIndex,
					err,
				)
			}
			domains[reference.Domain] =
				struct{}{}
		}
		if len(domains) < 2 {
			return fmt.Errorf(
				"conflict %d must reference at least two distinct domains",
				index,
			)
		}
	}

	for index, risk := range value.Risks {
		if strings.TrimSpace(risk) == "" {
			return fmt.Errorf(
				"risk %d is empty",
				index,
			)
		}
	}

	return nil
}

func validateReference(
	reference Reference,
	eventIDs map[string]struct{},
	fundamentalPresent bool,
) error {
	if reference.EventID != strings.TrimSpace(reference.EventID) {
		return errors.New("event_id must not contain surrounding whitespace")
	}
	switch reference.Domain {
	case DomainMarket,
		DomainTechnical:
		if strings.TrimSpace(
			reference.EventID,
		) != "" {
			return fmt.Errorf(
				"domain %q cannot carry event_id",
				reference.Domain,
			)
		}
	case DomainFundamental:
		if !fundamentalPresent {return errors.New("fundamental reference requires FundamentalContext")}
		if reference.EventID!=""{return errors.New("fundamental reference cannot carry event_id")}
	case DomainEvent:
		eventID := strings.TrimSpace(
			reference.EventID,
		)
		if eventID != "" {
			if _, ok := eventIDs[eventID]; !ok {
				return fmt.Errorf(
					"event_id %q does not exist in EventContext",
					eventID,
				)
			}
		}
	default:
		return fmt.Errorf(
			"invalid synthesis reference domain %q",
			reference.Domain,
		)
	}

	return nil
}
