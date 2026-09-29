package event

import (
	"errors"
	"fmt"
	"strings"
)

type ResearchPlan struct {
	Steps []ResearchPlanStep `json:"steps"`
}

type ResearchPlanStep struct {
	ID              string           `json:"id"`
	InformationNeed InformationNeed  `json:"information_need"`
	SourceType      SourceType       `json:"source_type"`
	Scope           AcquisitionScope `json:"scope"`
	Reason          string           `json:"reason"`
}

func (p ResearchPlan) Validate() error {
	if len(p.Steps) == 0 {
		return errors.New(
			"event research plan requires at least one step",
		)
	}

	seenIDs := make(map[string]struct{}, len(p.Steps))
	for index, step := range p.Steps {
		step.ID = strings.TrimSpace(step.ID)
		if step.ID == "" {
			return fmt.Errorf(
				"event research plan step %d id is required",
				index,
			)
		}
		if _, exists := seenIDs[step.ID]; exists {
			return fmt.Errorf(
				"event research plan step id %q is duplicated",
				step.ID,
			)
		}
		seenIDs[step.ID] = struct{}{}

		if step.InformationNeed == "" {
			return fmt.Errorf(
				"event research plan step %q information_need is required",
				step.ID,
			)
		}
		if step.SourceType == "" {
			return fmt.Errorf(
				"event research plan step %q source_type is required",
				step.ID,
			)
		}
		switch step.Scope {
		case AcquisitionScopeDirect,
			AcquisitionScopeCandidate:
		default:
			return fmt.Errorf(
				"event research plan step %q has invalid scope %q",
				step.ID,
				step.Scope,
			)
		}
		if strings.TrimSpace(step.Reason) == "" {
			return fmt.Errorf(
				"event research plan step %q reason is required",
				step.ID,
			)
		}
	}

	return nil
}

type ResearchPlanCompiler struct {
	OfficialDisclosureRetriever Retriever
	BreakingNewsRetriever       Retriever
	Selector                    EvidenceSelector
}

func (c ResearchPlanCompiler) Compile(
	root ResearchTask,
	plan ResearchPlan,
) (AcquisitionPlan, error) {
	if err := plan.Validate(); err != nil {
		return AcquisitionPlan{}, err
	}

	var acquisition AcquisitionPlan
	for _, researchStep := range plan.Steps {
		step, err := c.compileStep(
			root,
			researchStep,
		)
		if err != nil {
			return AcquisitionPlan{}, err
		}
		acquisition.Steps = append(
			acquisition.Steps,
			step,
		)
	}

	for _, step := range acquisition.Steps {
		if step.Scope != AcquisitionScopeCandidate {
			continue
		}
		if c.Selector == nil {
			return AcquisitionPlan{}, errors.New(
				"event research plan compiler requires an evidence selector for candidate steps",
			)
		}
		acquisition.Selector = c.Selector
		break
	}

	return acquisition, nil
}

func (c ResearchPlanCompiler) compileStep(
	root ResearchTask,
	step ResearchPlanStep,
) (AcquisitionStep, error) {
	switch {
	case step.InformationNeed == NeedOfficialDisclosure &&
		step.SourceType == SourceOfficial &&
		step.Scope == AcquisitionScopeDirect:
		if c.OfficialDisclosureRetriever == nil {
			return AcquisitionStep{}, fmt.Errorf(
				"event research plan step %q requires official disclosure retriever",
				step.ID,
			)
		}
		symbol := strings.TrimSpace(root.Symbol)
		if symbol == "" {
			return AcquisitionStep{}, fmt.Errorf(
				"event research plan step %q requires root symbol for official disclosure",
				step.ID,
			)
		}
		return AcquisitionStep{
			ID:        strings.TrimSpace(step.ID),
			Scope:     AcquisitionScopeDirect,
			Retriever: c.OfficialDisclosureRetriever,
			Task: ResearchTask{
				Symbol:          symbol,
				InformationNeed: NeedOfficialDisclosure,
				Requirements: ResearchRequirements{
					PITSafe:               root.Requirements.PITSafe,
					RequiredSourceTypes:   []SourceType{SourceOfficial},
					MinDocuments:          1,
					MinIndependentSources: 1,
					RequirePrimarySource:  true,
				},
			},
		}, nil

	case step.InformationNeed == NeedBreakingNews &&
		step.SourceType == SourceIndustryMedia &&
		step.Scope == AcquisitionScopeCandidate:
		if c.BreakingNewsRetriever == nil {
			return AcquisitionStep{}, fmt.Errorf(
				"event research plan step %q requires breaking news retriever",
				step.ID,
			)
		}
		return AcquisitionStep{
			ID:        strings.TrimSpace(step.ID),
			Scope:     AcquisitionScopeCandidate,
			Retriever: c.BreakingNewsRetriever,
			Task: ResearchTask{
				InformationNeed: NeedBreakingNews,
				Requirements: ResearchRequirements{
					PITSafe:               root.Requirements.PITSafe,
					RequiredSourceTypes:   []SourceType{SourceIndustryMedia},
					MinDocuments:          1,
					MinIndependentSources: 1,
				},
			},
		}, nil
	}

	return AcquisitionStep{}, fmt.Errorf(
		"event research plan step %q has unsupported combination: information_need=%q source_type=%q scope=%q",
		step.ID,
		step.InformationNeed,
		step.SourceType,
		step.Scope,
	)
}

type ReplanAction string

const (
	ReplanComplete ReplanAction = "complete"
	ReplanContinue ReplanAction = "continue"
)

type ReplanDecision struct {
	Action ReplanAction  `json:"action"`
	Reason string        `json:"reason"`
	Plan   *ResearchPlan `json:"plan,omitempty"`
}

func ValidateReplanDecision(
	result ResearchTaskResult,
	decision ReplanDecision,
) error {
	if strings.TrimSpace(decision.Reason) == "" {
		return errors.New(
			"event replan decision reason is required",
		)
	}

	switch decision.Action {
	case ReplanComplete:
		if result.Status != TaskSatisfied {
			return fmt.Errorf(
				"event replan cannot complete task with status %q",
				result.Status,
			)
		}
		if decision.Plan != nil {
			return errors.New(
				"event complete replan decision must not include a plan",
			)
		}
		return nil

	case ReplanContinue:
		if result.Status == TaskSatisfied {
			return errors.New(
				"event satisfied task must complete instead of continue",
			)
		}
		if decision.Plan == nil {
			return errors.New(
				"event continue replan decision requires a plan",
			)
		}
		if err := decision.Plan.Validate(); err != nil {
			return fmt.Errorf(
				"invalid continued event research plan: %w",
				err,
			)
		}
		return nil

	default:
		return fmt.Errorf(
			"event replan action %q is invalid",
			decision.Action,
		)
	}
}
