package planning

import (
	"Aegis/internal/capability/event"
	"errors"
	"fmt"
	"strings"
)

type ChannelBindings struct {
	OfficialDisclosure event.Retriever
	BreakingNews       event.Retriever
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

	compiled := event.AcquisitionPlan{
		Steps: make(
			[]event.AcquisitionStep,
			0,
			len(plan.Steps),
		),
		Selector: c.Channels.Selector,
	}

	for _, step := range plan.Steps {
		switch step.Need {
		case event.NeedOfficialDisclosure:
			if strings.TrimSpace(root.Symbol) == "" {
				return event.AcquisitionPlan{}, fmt.Errorf(
					"research plan step %q requires root symbol for official disclosure",
					step.ID,
				)
			}
			if c.Channels.OfficialDisclosure == nil {
				return event.AcquisitionPlan{}, fmt.Errorf(
					"research plan step %q requires official disclosure channel binding",
					step.ID,
				)
			}

			compiled.Steps = append(
				compiled.Steps,
				event.AcquisitionStep{
					ID:    step.ID,
					Scope: event.AcquisitionScopeDirect,
					Task: event.ResearchTask{
						Symbol:          root.Symbol,
						InformationNeed: event.NeedOfficialDisclosure,
					},
					Retriever: c.Channels.OfficialDisclosure,
				},
			)

		case event.NeedBreakingNews:
			if c.Channels.BreakingNews == nil {
				return event.AcquisitionPlan{}, fmt.Errorf(
					"research plan step %q requires breaking news channel binding",
					step.ID,
				)
			}

			compiled.Steps = append(
				compiled.Steps,
				event.AcquisitionStep{
					ID:    step.ID,
					Scope: event.AcquisitionScopeCandidate,
					Task: event.ResearchTask{
						// Broad breaking-news retrieval is intentionally
						// symbol-less. Root-symbol relevance is decided only
						// by EvidenceSelector after acquisition.
						Symbol:          "",
						InformationNeed: event.NeedBreakingNews,
					},
					Retriever: c.Channels.BreakingNews,
				},
			)

		default:
			return event.AcquisitionPlan{}, fmt.Errorf(
				"research plan step %q has unsupported information need %q",
				step.ID,
				step.Need,
			)
		}
	}

	return compiled, nil
}
