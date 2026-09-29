package planning

import (
	"Aegis/internal/capability/event"
	"errors"
	"fmt"
	"strings"
)

type ChannelBindings struct {
	OfficialDisclosure []event.Retriever
	BreakingNews       []event.Retriever
	Selector           event.EvidenceSelector
}

type Compiler struct {
	Channels ChannelBindings
}

func (c Compiler) Compile(
	root event.ResearchTask,
	plan *ResearchPlan,
) (event.AcquisitionPlan, error) {
	if err := plan.Validate(); err != nil {
		return event.AcquisitionPlan{}, err
	}

	root.ID = strings.TrimSpace(root.ID)
	if root.ID == "" {
		return event.AcquisitionPlan{}, errors.New(
			"research planner root task id is required",
		)
	}

	hasCandidate := false
	seenNeeds := make(
		map[event.InformationNeed]struct{},
		len(plan.Steps),
	)

	compiled := event.AcquisitionPlan{
		Steps:    make([]event.AcquisitionStep, 0),
		Selector: c.Channels.Selector,
	}

	for _, step := range plan.Steps {
		if _, duplicate := seenNeeds[
			step.Need,
		]; duplicate {
			// Planner semantics allow at most one logical step per evidence
			// need. A logical need can still fan out to every concrete
			// retriever bound to that channel.
			continue
		}
		seenNeeds[step.Need] = struct{}{}

		switch step.Need {
		case event.NeedOfficialDisclosure:
			if strings.TrimSpace(root.Symbol) == "" {
				return event.AcquisitionPlan{}, fmt.Errorf(
					"research plan step %q requires root symbol for official disclosure",
					step.ID,
				)
			}

			retrievers, err := validateChannelRetrievers(
				"official disclosure",
				c.Channels.OfficialDisclosure,
			)
			if err != nil {
				return event.AcquisitionPlan{}, fmt.Errorf(
					"research plan step %q: %w",
					step.ID,
					err,
				)
			}

			for _, retriever := range retrievers {
				compiled.Steps = append(
					compiled.Steps,
					event.AcquisitionStep{
						ID: providerStepID(
							step.ID,
							retriever.ID(),
							len(retrievers),
						),
						Scope: event.AcquisitionScopeDirect,
						Task: event.ResearchTask{
							Symbol:          root.Symbol,
							InformationNeed: event.NeedOfficialDisclosure,
						},
						Retriever: retriever,
					},
				)
			}

		case event.NeedBreakingNews:
			hasCandidate = true

			retrievers, err := validateChannelRetrievers(
				"breaking news",
				c.Channels.BreakingNews,
			)
			if err != nil {
				return event.AcquisitionPlan{}, fmt.Errorf(
					"research plan step %q: %w",
					step.ID,
					err,
				)
			}

			for _, retriever := range retrievers {
				compiled.Steps = append(
					compiled.Steps,
					event.AcquisitionStep{
						ID: providerStepID(
							step.ID,
							retriever.ID(),
							len(retrievers),
						),
						Scope: event.AcquisitionScopeCandidate,
						Task: event.ResearchTask{
							// Broad breaking-news retrieval is intentionally
							// symbol-less. Root-symbol relevance is decided only
							// by EvidenceSelector after all candidate providers
							// have contributed their documents.
							Symbol:          "",
							InformationNeed: event.NeedBreakingNews,
						},
						Retriever: retriever,
					},
				)
			}

		default:
			return event.AcquisitionPlan{}, fmt.Errorf(
				"research plan step %q has unsupported information need %q",
				step.ID,
				step.Need,
			)
		}
	}

	if hasCandidate && c.Channels.Selector == nil {
		return event.AcquisitionPlan{}, errors.New(
			"research plan with candidate evidence requires selector binding",
		)
	}

	return compiled, nil
}

func validateChannelRetrievers(
	channel string,
	retrievers []event.Retriever,
) ([]event.Retriever, error) {
	if len(retrievers) == 0 {
		return nil, fmt.Errorf(
			"%s channel requires at least one retriever binding",
			channel,
		)
	}

	validated := make(
		[]event.Retriever,
		0,
		len(retrievers),
	)
	seenIDs := make(
		map[string]struct{},
		len(retrievers),
	)

	for index, retriever := range retrievers {
		if retriever == nil {
			return nil, fmt.Errorf(
				"%s channel retriever %d is nil",
				channel,
				index,
			)
		}

		retrieverID := strings.TrimSpace(
			retriever.ID(),
		)
		if retrieverID == "" {
			return nil, fmt.Errorf(
				"%s channel retriever %d id is required",
				channel,
				index,
			)
		}
		if retriever.Method() == "" {
			return nil, fmt.Errorf(
				"%s channel retriever %q acquisition method is required",
				channel,
				retrieverID,
			)
		}
		if _, duplicate := seenIDs[retrieverID]; duplicate {
			return nil, fmt.Errorf(
				"%s channel retriever id %q is duplicated",
				channel,
				retrieverID,
			)
		}
		seenIDs[retrieverID] = struct{}{}
		validated = append(
			validated,
			retriever,
		)
	}

	return validated, nil
}

func providerStepID(
	baseID string,
	retrieverID string,
	providerCount int,
) string {
	if providerCount <= 1 {
		return baseID
	}
	return baseID + "@" + strings.TrimSpace(retrieverID)
}
