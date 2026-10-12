package planning

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var ResearchPlanToolInfo = schema.ToolInfo{
	Name: "research_plan",
	Desc: "Create the complete evidence-acquisition plan for the next research round. Official disclosures are direct evidence. Broad breaking-news feeds are candidate evidence and require later relevance selection.",
	ParamsOneOf: schema.NewParamsOneOfByParams(
		map[string]*schema.ParameterInfo{
			"steps": {
				Type:     schema.Array,
				Required: true,
				Desc:     "All complementary evidence channels needed in this research round. These are not fallback providers.",
				ElemInfo: &schema.ParameterInfo{
					Type: schema.Object,
					SubParams: map[string]*schema.ParameterInfo{
						"id": {
							Type:     schema.String,
							Required: true,
							Desc:     "Stable step identifier within this research round.",
						},
						"need": {
							Type:     schema.String,
							Required: true,
							Enum: []string{
								string("official_disclosure"),
								string("breaking_news"),
							},
							Desc: "Evidence need. Use official_disclosure for issuer/regulatory primary evidence and breaking_news for broad media candidates.",
						},
						"scope": {
							Type:     schema.String,
							Required: true,
							Enum: []string{
								string("direct"),
								string("candidate"),
							},
							Desc: "official_disclosure must be direct; breaking_news must be candidate.",
						},
						"reason": {
							Type:     schema.String,
							Required: false,
							Desc:     "用简体中文说明此证据渠道为何必要；保留所有字段名、类型值及 ID 原文。",
						},
					},
				},
			},
		},
	),
}

func NewPlanner(
	ctx context.Context,
	chatModel model.ToolCallingChatModel,
) (adk.Agent, error) {
	if chatModel == nil {
		return nil, errors.New(
			"research planner chat model is required",
		)
	}

	return planexecute.NewPlanner(
		ctx,
		&planexecute.PlannerConfig{
			ToolCallingChatModel: &validatedResearchToolModel{
				base: chatModel,
				phase: "planner",
			},
			ToolInfo:             &ResearchPlanToolInfo,
			GenInputFn:           researchPlannerInput,
			NewPlan: func(context.Context) planexecute.Plan {
				return &ResearchPlan{}
			},
		},
	)
}

func NewReplanner(
	ctx context.Context,
	chatModel model.ToolCallingChatModel,
) (adk.Agent, error) {
	if chatModel == nil {
		return nil, errors.New(
			"research replanner chat model is required",
		)
	}

	return planexecute.NewReplanner(
		ctx,
		&planexecute.ReplannerConfig{
			ChatModel: &validatedResearchToolModel{
				base: chatModel,
				phase: "replanner",
			},
			PlanTool:  &ResearchPlanToolInfo,
			NewPlan: func(context.Context) planexecute.Plan {
				return &ResearchPlan{}
			},
		},
	)
}

func researchPlannerInput(
	_ context.Context,
	userInput []adk.Message,
) ([]adk.Message, error) {
	messages := make(
		[]adk.Message,
		0,
		len(userInput)+1,
	)
	messages = append(
		messages,
		schema.SystemMessage(
			`You are the research planning layer for Aegis.

Create exactly one structured ResearchPlan for the next evidence-acquisition round.
Use Simplified Chinese for any generated human-readable task explanations or plan reasoning; keep tool field names, evidence scopes, IDs and enum strings intact.

Rules:
- Plan complementary evidence needs, not fallback providers.
- official_disclosure must use scope=direct.
- breaking_news must use scope=candidate.
- Do not claim that broad breaking-news feeds are symbol-aware.
- Do not perform relevance filtering in the plan.
- Do not invent provider names, URLs, crawler details, or tool implementation.
- Keep the plan minimal: include only evidence channels justified by the objective and known gaps.
- Include each evidence need at most once in a round. Do not create multiple official_disclosure steps or multiple breaking_news steps with different wording.
- The plan is compiled later into deterministic Aegis AcquisitionPlan steps.`,
		),
	)
	messages = append(messages, userInput...)
	return messages, nil
}
