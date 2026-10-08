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

// validateModelAction runs the underlying model using its native protocol,
// then checks the completed tool call before the Eino planner can consume it.
// A validation retry reuses the original research request with concise feedback.
func (m *validatedResearchToolModel) validateModelAction(
	ctx context.Context,
	input []*schema.Message,
	invoke func(context.Context, []*schema.Message) (*schema.Message, error),
) (*schema.Message, error) {
	if m == nil || m.base == nil {
		return nil, errors.New("research planning model is not initialized")
	}
	messages := append([]*schema.Message(nil), input...)
	for attempt := 1; attempt <= maxResearchToolAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err := invoke(ctx, messages)
		if err != nil {
			return nil, fmt.Errorf("%s model request: %w", m.phase, err)
		}
		if err := validateResearchToolAction(m.phase, response); err != nil {
			if attempt == maxResearchToolAttempts {
				return nil, fmt.Errorf("%s returned invalid planning action after %d attempts: %w",
					m.phase, attempt, err)
			}
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

func (m *validatedResearchToolModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.Message, error) {
	return m.validateModelAction(ctx, input,
		func(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
			return m.base.Generate(ctx, messages, opts...)
		})
}

// The Eino PlanExecute graph consumes the native Stream API. Preserve that
// provider contract; collect the whole tool call before validating or
// retrying, then return a single validated message stream to Eino.
func (m *validatedResearchToolModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	response, err := m.validateModelAction(ctx, input,
		func(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
			stream, err := m.base.Stream(ctx, messages, opts...)
			if err != nil {
				return nil, err
			}
			if stream == nil {
				return nil, errors.New("research planning stream is nil")
			}
			defer stream.Close()
			return schema.ConcatMessageStream(stream)
		})
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
