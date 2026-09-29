package planning

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeToolCallingChatModel struct {
	tools []*schema.ToolInfo
}

func (m *fakeToolCallingChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return nil, errors.New("not implemented")
}

func (m *fakeToolCallingChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("not implemented")
}

func (m *fakeToolCallingChatModel) WithTools(
	tools []*schema.ToolInfo,
) (model.ToolCallingChatModel, error) {
	m.tools = append(
		[]*schema.ToolInfo(nil),
		tools...,
	)
	return m, nil
}

func TestResearchPlanToolInfo(t *testing.T) {
	if ResearchPlanToolInfo.Name != "research_plan" {
		t.Fatalf(
			"tool name = %q",
			ResearchPlanToolInfo.Name,
		)
	}

	jsonSchema, err := ResearchPlanToolInfo.
		ParamsOneOf.
		ToJSONSchema()
	if err != nil {
		t.Fatalf(
			"ToJSONSchema() error = %v",
			err,
		)
	}
	if jsonSchema == nil {
		t.Fatal("research plan tool schema is nil")
	}
}

func TestNewPlannerAndReplanner(t *testing.T) {
	chatModel := &fakeToolCallingChatModel{}

	planner, err := NewPlanner(
		context.Background(),
		chatModel,
	)
	if err != nil {
		t.Fatalf(
			"NewPlanner() error = %v",
			err,
		)
	}
	if planner == nil {
		t.Fatal("planner is nil")
	}
	if len(chatModel.tools) != 1 ||
		chatModel.tools[0] != &ResearchPlanToolInfo {
		t.Fatalf(
			"planner tools = %v, want ResearchPlanToolInfo",
			chatModel.tools,
		)
	}

	chatModel.tools = nil
	replanner, err := NewReplanner(
		context.Background(),
		chatModel,
	)
	if err != nil {
		t.Fatalf(
			"NewReplanner() error = %v",
			err,
		)
	}
	if replanner == nil {
		t.Fatal("replanner is nil")
	}
	if len(chatModel.tools) != 2 {
		t.Fatalf(
			"replanner tools = %d, want 2",
			len(chatModel.tools),
		)
	}
	if chatModel.tools[0] != &ResearchPlanToolInfo {
		t.Fatal("replanner must use ResearchPlanToolInfo as plan tool")
	}
}

func TestPlannerRequiresChatModel(t *testing.T) {
	if _, err := NewPlanner(
		context.Background(),
		nil,
	); err == nil {
		t.Fatal("planner should require chat model")
	}

	if _, err := NewReplanner(
		context.Background(),
		nil,
	); err == nil {
		t.Fatal("replanner should require chat model")
	}
}
