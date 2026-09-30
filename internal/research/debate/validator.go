package debate

import (
	"Aegis/internal/research/synthesis"
	"errors"
	"fmt"
	"strings"
)

func ValidateState(state *State, artifact *synthesis.Artifact) error {
	if state == nil {
		return errors.New("debate state is nil")
	}
	if err := synthesis.Validate(artifact); err != nil {
		return fmt.Errorf("invalid research artifact: %w", err)
	}
	symbol := strings.TrimSpace(state.Symbol)
	if symbol == "" {
		return errors.New("debate symbol is required")
	}
	if symbol != strings.TrimSpace(artifact.Symbol) {
		return fmt.Errorf("debate symbol %q does not match research artifact symbol %q", symbol, artifact.Symbol)
	}

	argumentIDs := make(map[string]struct{})
	for index, turn := range state.Turns {
		if err := validateTurn(turn, artifact, argumentIDs); err != nil {
			return fmt.Errorf("turn %d: %w", index, err)
		}
		for _, argument := range turn.Arguments {
			argumentIDs[strings.TrimSpace(argument.ID)] = struct{}{}
		}
	}
	return nil
}

func validateTurn(turn Turn, artifact *synthesis.Artifact, priorArgumentIDs map[string]struct{}) error {
	if turn.Round <= 0 {
		return errors.New("round must be positive")
	}
	if turn.Side != SideBull && turn.Side != SideBear {
		return fmt.Errorf("invalid side %q", turn.Side)
	}
	if strings.TrimSpace(turn.Thesis) == "" {
		return errors.New("thesis is required")
	}
	if turn.Confidence < 0 || turn.Confidence > 1 {
		return fmt.Errorf("confidence out of range: %f", turn.Confidence)
	}
	if len(turn.Arguments) == 0 {
		return errors.New("at least one argument is required")
	}

	localIDs := make(map[string]struct{})
	for index, argument := range turn.Arguments {
		id := strings.TrimSpace(argument.ID)
		if id == "" {
			return fmt.Errorf("argument %d id is required", index)
		}
		if _, ok := priorArgumentIDs[id]; ok {
			return fmt.Errorf("argument %d id %q already exists", index, id)
		}
		if _, ok := localIDs[id]; ok {
			return fmt.Errorf("argument %d id %q is duplicated in turn", index, id)
		}
		localIDs[id] = struct{}{}
		if strings.TrimSpace(argument.Claim) == "" {
			return fmt.Errorf("argument %d claim is required", index)
		}
		if len(argument.References) == 0 {
			return fmt.Errorf("argument %d requires at least one reference", index)
		}
		for refIndex, ref := range argument.References {
			if err := validateReference(ref, artifact); err != nil {
				return fmt.Errorf("argument %d reference %d: %w", index, refIndex, err)
			}
		}
	}

	for index, rebuttal := range turn.Rebuttals {
		if strings.TrimSpace(rebuttal.Claim) == "" {
			return fmt.Errorf("rebuttal %d claim is required", index)
		}
		target := strings.TrimSpace(rebuttal.TargetArgumentID)
		if target == "" {
			return fmt.Errorf("rebuttal %d target_argument_id is required", index)
		}
		if _, ok := priorArgumentIDs[target]; !ok {
			return fmt.Errorf("rebuttal %d target_argument_id %q does not reference a prior argument", index, target)
		}
		if len(rebuttal.References) == 0 {
			return fmt.Errorf("rebuttal %d requires at least one reference", index)
		}
		for refIndex, ref := range rebuttal.References {
			if err := validateReference(ref, artifact); err != nil {
				return fmt.Errorf("rebuttal %d reference %d: %w", index, refIndex, err)
			}
		}
	}
	return nil
}

func validateReference(ref synthesis.Reference, artifact *synthesis.Artifact) error {
	switch ref.Domain {
	case synthesis.DomainMarket, synthesis.DomainTechnical:
		if strings.TrimSpace(ref.EventID) != "" {
			return fmt.Errorf("domain %q cannot carry event_id", ref.Domain)
		}
	case synthesis.DomainEvent:
		eventID := strings.TrimSpace(ref.EventID)
		if eventID == "" {
			return nil
		}
		for _, item := range artifact.Event.Events {
			if item.ID == eventID {
				return nil
			}
		}
		return fmt.Errorf("event_id %q does not exist in ResearchArtifact", eventID)
	default:
		return fmt.Errorf("invalid reference domain %q", ref.Domain)
	}
	return nil
}
