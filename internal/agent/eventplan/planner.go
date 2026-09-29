package eventplan

import (
	eventcap "Aegis/internal/capability/event"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const plannerInstruction = `You are the research-planning layer for Aegis event evidence acquisition.

You decide WHAT evidence should be researched next. You do not decide HOW a provider retrieves it, and you must not invent provider names, URLs, tools, or source semantics.

Return exactly one JSON object matching:
{
  "steps": [
    {
      "id": "short-stable-step-id",
      "information_need": "official_disclosure|breaking_news",
      "source_type": "official|industry_media",
      "scope": "direct|candidate",
      "reason": "why this evidence is needed"
    }
  ]
}

Only these combinations are executable:
1. official_disclosure + official + direct
2. breaking_news + industry_media + candidate

Semantics:
- official disclosure is symbol-aware primary evidence.
- breaking news is a broad candidate feed and MUST remain candidate; relevance is decided later against the root research task.
- Choose complementary evidence required by the root ResearchRequirements.
- Do not claim evidence already exists. You are creating an acquisition plan.
- Never output markdown, prose, comments, or code fences outside the JSON object.`

const replannerInstruction = `You are the research-replanning layer for Aegis event evidence acquisition.

You receive:
- the root ResearchTask,
- the previous ResearchPlan,
- the deterministic ResearchTaskResult including Status and Gaps.

Return exactly one JSON object in one of these forms:

COMPLETE:
{
  "action": "complete",
  "reason": "why the evidence contract is satisfied"
}

CONTINUE:
{
  "action": "continue",
  "reason": "which deterministic gaps still need evidence",
  "plan": {
    "steps": [
      {
        "id": "short-stable-step-id",
        "information_need": "official_disclosure|breaking_news",
        "source_type": "official|industry_media",
        "scope": "direct|candidate",
        "reason": "how this step addresses a reported gap"
      }
    ]
  }
}

Rules:
- You may choose complete only when ResearchTaskResult.status is "satisfied".
- For "partial" or "unsatisfied", choose continue and address reported gaps.
- Plan only remaining evidence work; do not repeat completed work without a reported reason.
- Only these executable combinations are allowed:
  1. official_disclosure + official + direct
  2. breaking_news + industry_media + candidate
- breaking_news must remain candidate.
- Never output markdown, prose, comments, or code fences outside the JSON object.`

type Config struct {
	Model model.BaseModel[*schema.AgenticMessage]

	MaxIterations int
}

type Planner struct {
	plannerRunner   *adk.TypedRunner[*schema.AgenticMessage]
	replannerRunner *adk.TypedRunner[*schema.AgenticMessage]
}

func New(
	ctx context.Context,
	cfg Config,
) (*Planner, error) {
	if cfg.Model == nil {
		return nil, errors.New(
			"event research planner model is required",
		)
	}

	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 1
	}

	plannerAgent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](
		ctx,
		&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:          "event_research_planner",
			Description:   "Plans which event evidence channels should be researched next.",
			Instruction:   plannerInstruction,
			Model:         cfg.Model,
			MaxIterations: maxIterations,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create event research planner agent: %w",
			err,
		)
	}

	replannerAgent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](
		ctx,
		&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:          "event_research_replanner",
			Description:   "Uses deterministic evidence gaps to decide complete or produce the next research plan.",
			Instruction:   replannerInstruction,
			Model:         cfg.Model,
			MaxIterations: maxIterations,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create event research replanner agent: %w",
			err,
		)
	}

	return &Planner{
		plannerRunner: adk.NewTypedRunner(
			adk.TypedRunnerConfig[*schema.AgenticMessage]{
				Agent:           plannerAgent,
				EnableStreaming: false,
			},
		),
		replannerRunner: adk.NewTypedRunner(
			adk.TypedRunnerConfig[*schema.AgenticMessage]{
				Agent:           replannerAgent,
				EnableStreaming: false,
			},
		),
	}, nil
}

func (p *Planner) Plan(
	ctx context.Context,
	task eventcap.ResearchTask,
	opts ...adk.AgentRunOption,
) (*eventcap.ResearchPlan, error) {
	if p == nil || p.plannerRunner == nil {
		return nil, errors.New(
			"event research planner is not initialized",
		)
	}
	if strings.TrimSpace(task.ID) == "" {
		return nil, errors.New(
			"event research planner task id is required",
		)
	}
	if task.InformationNeed != eventcap.NeedEventEvidence {
		return nil, fmt.Errorf(
			"event research planner requires information need %q, got %q",
			eventcap.NeedEventEvidence,
			task.InformationNeed,
		)
	}
	if task.Requirements.PITSafe && task.AsOf.IsZero() {
		return nil, errors.New(
			"event research planner as_of is required for PIT-safe task",
		)
	}

	payload, err := json.Marshal(struct {
		Task eventcap.ResearchTask `json:"task"`
	}{
		Task: task,
	})
	if err != nil {
		return nil, fmt.Errorf(
			"marshal event research planner input: %w",
			err,
		)
	}

	raw, err := runJSONAgent(
		ctx,
		p.plannerRunner,
		string(payload),
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"event research planner run: %w",
			err,
		)
	}

	var plan eventcap.ResearchPlan
	if err := json.Unmarshal(
		[]byte(raw),
		&plan,
	); err != nil {
		return nil, fmt.Errorf(
			"invalid event research plan JSON: %w: output=%q",
			err,
			raw,
		)
	}
	if err := plan.Validate(); err != nil {
		return nil, fmt.Errorf(
			"invalid event research plan: %w",
			err,
		)
	}

	return &plan, nil
}

func (p *Planner) Replan(
	ctx context.Context,
	task eventcap.ResearchTask,
	previous eventcap.ResearchPlan,
	result eventcap.ResearchTaskResult,
	opts ...adk.AgentRunOption,
) (*eventcap.ReplanDecision, error) {
	if p == nil || p.replannerRunner == nil {
		return nil, errors.New(
			"event research replanner is not initialized",
		)
	}
	if strings.TrimSpace(task.ID) == "" {
		return nil, errors.New(
			"event research replanner task id is required",
		)
	}
	if err := previous.Validate(); err != nil {
		return nil, fmt.Errorf(
			"invalid previous event research plan: %w",
			err,
		)
	}
	if strings.TrimSpace(result.TaskID) != "" &&
		result.TaskID != task.ID {
		return nil, fmt.Errorf(
			"event research result task id %q does not match root task id %q",
			result.TaskID,
			task.ID,
		)
	}

	payload, err := json.Marshal(struct {
		Task         eventcap.ResearchTask       `json:"task"`
		PreviousPlan eventcap.ResearchPlan       `json:"previous_plan"`
		Result       eventcap.ResearchTaskResult `json:"result"`
	}{
		Task:         task,
		PreviousPlan: previous,
		Result:       result,
	})
	if err != nil {
		return nil, fmt.Errorf(
			"marshal event research replanner input: %w",
			err,
		)
	}

	raw, err := runJSONAgent(
		ctx,
		p.replannerRunner,
		string(payload),
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"event research replanner run: %w",
			err,
		)
	}

	var decision eventcap.ReplanDecision
	if err := json.Unmarshal(
		[]byte(raw),
		&decision,
	); err != nil {
		return nil, fmt.Errorf(
			"invalid event replan decision JSON: %w: output=%q",
			err,
			raw,
		)
	}
	if err := eventcap.ValidateReplanDecision(
		result,
		decision,
	); err != nil {
		return nil, fmt.Errorf(
			"invalid event replan decision: %w",
			err,
		)
	}

	return &decision, nil
}

func runJSONAgent(
	ctx context.Context,
	runner *adk.TypedRunner[*schema.AgenticMessage],
	query string,
	opts ...adk.AgentRunOption,
) (string, error) {
	iter := runner.Query(
		ctx,
		query,
		opts...,
	)

	var output string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return "", event.Err
		}
		if event.Output == nil ||
			event.Output.MessageOutput == nil ||
			event.Output.MessageOutput.Message == nil {
			continue
		}

		text := assistantText(
			event.Output.MessageOutput.Message,
		)
		if text != "" {
			output = text
		}
	}

	if strings.TrimSpace(output) == "" {
		return "", errors.New(
			"event research planning agent returned empty output",
		)
	}

	return strings.TrimSpace(output), nil
}

func assistantText(
	msg *schema.AgenticMessage,
) string {
	if msg == nil ||
		msg.Role != schema.AgenticRoleTypeAssistant {
		return ""
	}

	for _, block := range msg.ContentBlocks {
		if block != nil &&
			block.Type == schema.ContentBlockTypeFunctionToolCall &&
			block.FunctionToolCall != nil {
			return ""
		}
	}

	var b strings.Builder
	for _, block := range msg.ContentBlocks {
		if block == nil ||
			block.Type != schema.ContentBlockTypeAssistantGenText ||
			block.AssistantGenText == nil {
			continue
		}
		b.WriteString(
			block.AssistantGenText.Text,
		)
	}
	return b.String()
}
