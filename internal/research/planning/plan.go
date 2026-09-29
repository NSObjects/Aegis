package planning

import (
	"Aegis/internal/capability/event"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
)

const executeResearchPlanStep = "execute_research_plan"

type ResearchPlanStep struct {
	ID     string                 `json:"id"`
	Need   event.InformationNeed  `json:"need"`
	Scope  event.AcquisitionScope `json:"scope"`
	Reason string                 `json:"reason,omitempty"`
}

// ResearchPlan describes one complete evidence-acquisition round.
// Eino plan-execute-replan iterates at the research-round level; individual
// retrievers are compiled into one event.AcquisitionPlan below this boundary.
type ResearchPlan struct {
	Steps []ResearchPlanStep `json:"steps"`
}

var _ planexecute.Plan = (*ResearchPlan)(nil)

func (p *ResearchPlan) FirstStep() string {
	if p == nil || len(p.Steps) == 0 {
		return ""
	}
	return executeResearchPlanStep
}

func (p *ResearchPlan) MarshalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	type planAlias ResearchPlan
	return json.Marshal((*planAlias)(p))
}

func (p *ResearchPlan) UnmarshalJSON(data []byte) error {
	type planAlias ResearchPlan

	var decoded planAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf(
			"decode research plan: %w",
			err,
		)
	}

	*p = ResearchPlan(decoded)
	if err := p.Validate(); err != nil {
		return err
	}

	return nil
}

func (p *ResearchPlan) Validate() error {
	if p == nil {
		return errors.New(
			"research plan is required",
		)
	}
	if len(p.Steps) == 0 {
		return errors.New(
			"research plan requires at least one step",
		)
	}

	seenIDs := make(
		map[string]struct{},
		len(p.Steps),
	)

	for index := range p.Steps {
		step := &p.Steps[index]
		step.ID = strings.TrimSpace(step.ID)
		step.Reason = strings.TrimSpace(
			step.Reason,
		)

		if step.ID == "" {
			return fmt.Errorf(
				"research plan step %d id is required",
				index,
			)
		}
		if _, duplicate := seenIDs[step.ID]; duplicate {
			return fmt.Errorf(
				"research plan step id %q is duplicated",
				step.ID,
			)
		}
		seenIDs[step.ID] = struct{}{}

		switch step.Need {
		case event.NeedOfficialDisclosure,
			event.NeedBreakingNews:
		default:
			return fmt.Errorf(
				"research plan step %q has unsupported information need %q",
				step.ID,
				step.Need,
			)
		}

		switch step.Scope {
		case event.AcquisitionScopeDirect,
			event.AcquisitionScopeCandidate:
		default:
			return fmt.Errorf(
				"research plan step %q has unsupported acquisition scope %q",
				step.ID,
				step.Scope,
			)
		}

		switch step.Need {
		case event.NeedOfficialDisclosure:
			if step.Scope != event.AcquisitionScopeDirect {
				return fmt.Errorf(
					"research plan step %q official disclosure must use direct scope",
					step.ID,
				)
			}
		case event.NeedBreakingNews:
			if step.Scope != event.AcquisitionScopeCandidate {
				return fmt.Errorf(
					"research plan step %q breaking news must use candidate scope",
					step.ID,
				)
			}
		}
	}

	return nil
}
