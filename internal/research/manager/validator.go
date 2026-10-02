package manager

import (
	"Aegis/internal/research/debate"
	"Aegis/internal/research/synthesis"
	"errors"
	"fmt"
	"strings"
)

func Validate(plan *ResearchPlan, artifact *synthesis.Artifact, state *debate.State) error {
	if plan == nil {
		return errors.New("research plan is nil")
	}
	if err := synthesis.Validate(artifact); err != nil {
		return fmt.Errorf("invalid research artifact: %w", err)
	}
	if err := debate.ValidateState(state, artifact); err != nil {
		return fmt.Errorf("invalid debate state: %w", err)
	}
	symbol := strings.TrimSpace(plan.Symbol)
	if symbol == "" {
		return errors.New("research plan symbol is required")
	}
	if symbol != strings.TrimSpace(artifact.Symbol) {
		return fmt.Errorf("research plan symbol %q does not match research artifact symbol %q", symbol, artifact.Symbol)
	}
	switch plan.Conclusion {
	case ConclusionBullLean, ConclusionBearLean, ConclusionBalanced, ConclusionInsufficientEvidence:
	default:
		return fmt.Errorf("invalid research conclusion %q", plan.Conclusion)
	}
	if strings.TrimSpace(plan.Summary) == "" {
		return errors.New("research plan summary is required")
	}
	if plan.Confidence < 0 || plan.Confidence > 1 {
		return fmt.Errorf("confidence out of range: %f", plan.Confidence)
	}

	argumentIDs := map[string]struct{}{}
	for _, turn := range state.Turns {
		for _, argument := range turn.Arguments {
			argumentIDs[strings.TrimSpace(argument.ID)] = struct{}{}
		}
	}

	seen := map[string]string{}
	if err := validateArgumentIDs(plan.AcceptedArgumentIDs, "accepted_argument_ids", argumentIDs, seen); err != nil {
		return err
	}
	if err := validateArgumentIDs(plan.UnresolvedArgumentIDs, "unresolved_argument_ids", argumentIDs, seen); err != nil {
		return err
	}
	for i, insight := range plan.Opportunities {
		if err := validateInsight(insight, artifact); err != nil {
			return fmt.Errorf("opportunity %d: %w", i, err)
		}
	}
	for i, insight := range plan.Risks {
		if err := validateInsight(insight, artifact); err != nil {
			return fmt.Errorf("risk %d: %w", i, err)
		}
	}
	for i, constraint := range plan.TradingConstraints {
		if strings.TrimSpace(constraint) == "" {
			return fmt.Errorf("trading constraint %d is empty", i)
		}
	}
	return nil
}

func validateArgumentIDs(ids []string, field string, known map[string]struct{}, seen map[string]string) error {
	for i, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			return fmt.Errorf("%s[%d] is empty", field, i)
		}
		if _, ok := known[id]; !ok {
			return fmt.Errorf("%s[%d] %q does not exist in debate", field, i, id)
		}
		if prior, ok := seen[id]; ok {
			return fmt.Errorf("argument id %q appears in both %s and %s", id, prior, field)
		}
		seen[id] = field
	}
	return nil
}

func validateInsight(insight Insight, artifact *synthesis.Artifact) error {
	if strings.TrimSpace(insight.Claim) == "" {
		return errors.New("claim is required")
	}
	if len(insight.References) == 0 {
		return errors.New("at least one reference is required")
	}
	for i, ref := range insight.References {
		if err := validateReference(ref, artifact); err != nil {
			return fmt.Errorf("reference %d: %w", i, err)
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
