package planning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const maxResearchToolAttempts = 2

// validatedResearchToolModel guards the Eino PlanExecute model boundary.
// planexecute decodes the first tool call without a chance to correct invalid
// tool arguments. A failed schema decision is not a successful research plan:
// give the model one bounded opportunity to produce an actual valid action.
// Never invent plan steps or convert an empty plan into a successful response.
type validatedResearchToolModel struct {
	base  model.ToolCallingChatModel
	phase string // planner or replanner
}

var _ model.ToolCallingChatModel = (*validatedResearchToolModel)(nil)

func (m *validatedResearchToolModel) WithTools(
	tools []*schema.ToolInfo,
) (model.ToolCallingChatModel, error) {
	if m == nil || m.base == nil {
		return nil, errors.New("research planning model is not initialized")
	}
	bound, err := m.base.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &validatedResearchToolModel{base: bound, phase: m.phase}, nil
}

func (m *validatedResearchToolModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.Message, error) {
	if m == nil || m.base == nil {
		return nil, errors.New("research planning model is not initialized")
	}
	messages := append([]*schema.Message(nil), input...)
	for attempt := 1; attempt <= maxResearchToolAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err := m.base.Generate(ctx, messages, opts...)
		if err != nil {
			// Retrying provider errors may duplicate requests without repairing
			// a malformed action, so only schema errors are retried.
			return nil, fmt.Errorf("%s model request: %w", m.phase, err)
		}
		if err := validateResearchToolAction(m.phase, response); err != nil {
			if attempt == maxResearchToolAttempts {
				return nil, fmt.Errorf("%s returned invalid planning action after %d attempts: %w",
					m.phase, attempt, err)
			}
			// No model-supplied plan is silently changed. Ask for a NEW tool
			// call, preserving the original research context.
			messages = append(append([]*schema.Message(nil), input...),
				schema.UserMessage(fmt.Sprintf(
					"Your previous structured tool action did not satisfy the research contract: %s. "+
						"If additional evidence is needed, call research_plan with at least one fully valid step "+
						"(official_disclosure/direct or breaking_news/candidate). "+
						"If the objective is already achieved, use respond with a nonempty final response "+
						"(only if that tool is available). Do not invent retrieval results or provider names.",
					err,
				)))
			continue
		}
		return response, nil
	}
	return nil, errors.New("research planning retry exhausted")
}

// PlanExecute itself uses Stream even when called from a non-streaming agent.
// Collect the model response before the Eino parser sees it, so no unvalidated
// partial tool call escapes during this planning phase.
func (m *validatedResearchToolModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	response, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{response}), nil
}

func validateResearchToolAction(phase string, response *schema.Message) error {
	if response == nil || len(response.ToolCalls) != 1 {
		return errors.New("exactly one structured research action tool call is required")
	}
	call := response.ToolCalls[0].Function
	switch call.Name {
	case ResearchPlanToolInfo.Name:
		var plan ResearchPlan
		if err := json.Unmarshal([]byte(call.Arguments), &plan); err != nil {
			return fmt.Errorf("invalid research_plan arguments: %w", err)
		}
		return nil

	case "respond":
		if phase != "replanner" {
			return errors.New("planner must produce a research plan before execution")
		}
		var output struct {
			Response string `json:"response"`
		}
		decoder := json.NewDecoder(strings.NewReader(call.Arguments))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&output); err != nil {
			return fmt.Errorf("invalid respond arguments: %w", err)
		}
		if strings.TrimSpace(output.Response) == "" {
			return errors.New("respond requires a nonempty final response")
		}
		return nil
	default:
		return fmt.Errorf("unexpected %s tool %q", phase, call.Name)
	}
}
