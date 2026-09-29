package llm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type adapterAgenticModel struct {
	inputs      [][]*schema.AgenticMessage
	options     []*model.Options
	response    *schema.AgenticMessage
	err         error
	generateN   int
}

func (m *adapterAgenticModel) Generate(
	_ context.Context,
	input []*schema.AgenticMessage,
	opts ...model.Option,
) (*schema.AgenticMessage, error) {
	m.generateN++
	m.inputs = append(
		m.inputs,
		append(
			[]*schema.AgenticMessage(nil),
			input...,
		),
	)
	m.options = append(
		m.options,
		model.GetCommonOptions(nil, opts...),
	)
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *adapterAgenticModel) Stream(
	context.Context,
	[]*schema.AgenticMessage,
	...model.Option,
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	return nil, errors.New(
		"adapter must bridge through Generate",
	)
}

func TestAgenticToolCallingAdapterGenerate(t *testing.T) {
	base := &adapterAgenticModel{
		response: &schema.AgenticMessage{
			Role: schema.AgenticRoleTypeAssistant,
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(
					&schema.FunctionToolCall{
						CallID:    "call-plan",
						Name:      "research_plan",
						Arguments: `{"steps":[{"id":"official"}]}`,
					},
				),
			},
		},
	}

	adapter, err := NewAgenticToolCallingAdapter(base)
	if err != nil {
		t.Fatal(err)
	}
	withToolsAny, err := adapter.WithTools(
		[]*schema.ToolInfo{
			{
				Name: "research_plan",
				Desc: "plan",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	withTools := withToolsAny.(*AgenticToolCallingAdapter)

	result, err := withTools.Generate(
		context.Background(),
		[]*schema.Message{
			schema.SystemMessage("system instruction"),
			schema.UserMessage("plan evidence"),
		},
		model.WithToolChoice(
			schema.ToolChoiceForced,
		),
	)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf(
			"tool calls = %d, want 1",
			len(result.ToolCalls),
		)
	}
	if result.ToolCalls[0].Function.Name !=
		"research_plan" {
		t.Fatalf(
			"tool = %+v",
			result.ToolCalls[0],
		)
	}
	if result.ToolCalls[0].Function.Arguments !=
		`{"steps":[{"id":"official"}]}` {
		t.Fatalf(
			"arguments = %q",
			result.ToolCalls[0].Function.Arguments,
		)
	}

	if len(base.inputs) != 1 ||
		len(base.inputs[0]) != 2 {
		t.Fatalf(
			"agentic input = %+v",
			base.inputs,
		)
	}
	if base.inputs[0][0].Role !=
		schema.AgenticRoleTypeSystem ||
		base.inputs[0][1].Role !=
			schema.AgenticRoleTypeUser {
		t.Fatalf(
			"roles = %q / %q",
			base.inputs[0][0].Role,
			base.inputs[0][1].Role,
		)
	}

	opts := base.options[0]
	if len(opts.Tools) != 1 ||
		opts.Tools[0].Name != "research_plan" {
		t.Fatalf(
			"tools = %+v",
			opts.Tools,
		)
	}
	if opts.AgenticToolChoice == nil ||
		opts.AgenticToolChoice.Type !=
			schema.ToolChoiceForced {
		t.Fatalf(
			"agentic tool choice = %+v",
			opts.AgenticToolChoice,
		)
	}
}

func TestAgenticToolCallingAdapterWithToolsIsImmutable(
	t *testing.T,
) {
	base := &adapterAgenticModel{
		response: schema.UserAgenticMessage(
			"unused",
		),
	}
	adapter, err := NewAgenticToolCallingAdapter(base)
	if err != nil {
		t.Fatal(err)
	}

	firstAny, err := adapter.WithTools(
		[]*schema.ToolInfo{{Name: "first"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	secondAny, err := adapter.WithTools(
		[]*schema.ToolInfo{{Name: "second"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	first := firstAny.(*AgenticToolCallingAdapter)
	second := secondAny.(*AgenticToolCallingAdapter)
	if len(adapter.tools) != 0 {
		t.Fatalf(
			"base tools mutated: %+v",
			adapter.tools,
		)
	}
	if first.tools[0].Name != "first" ||
		second.tools[0].Name != "second" {
		t.Fatalf(
			"derived tools first=%+v second=%+v",
			first.tools,
			second.tools,
		)
	}
}

func TestAgenticToolCallingAdapterStreamUsesNormalizedGenerate(
	t *testing.T,
) {
	base := &adapterAgenticModel{
		response: &schema.AgenticMessage{
			Role: schema.AgenticRoleTypeAssistant,
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(
					&schema.FunctionToolCall{
						CallID:    "call-1",
						Name:      "plan",
						Arguments: `{"steps":[]}`,
					},
				),
			},
		},
	}
	adapter, err := NewAgenticToolCallingAdapter(base)
	if err != nil {
		t.Fatal(err)
	}

	stream, err := adapter.Stream(
		context.Background(),
		[]*schema.Message{
			schema.UserMessage("plan"),
		},
	)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	result, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if base.generateN != 1 {
		t.Fatalf(
			"Generate calls = %d, want 1",
			base.generateN,
		)
	}
	if len(result.ToolCalls) != 1 ||
		result.ToolCalls[0].Function.Name != "plan" {
		t.Fatalf(
			"stream result = %+v",
			result,
		)
	}
}

func TestAgenticToolCallingAdapterRejectsUnsupportedClassicInput(
	t *testing.T,
) {
	adapter, err := NewAgenticToolCallingAdapter(
		&adapterAgenticModel{},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = adapter.Generate(
		context.Background(),
		[]*schema.Message{
			{
				Role: schema.User,
				UserInputMultiContent: []schema.MessageInputPart{
					{
						Type: schema.ChatMessagePartTypeText,
						Text: "multimodal shape",
					},
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"text-only classic messages",
		) {
		t.Fatalf(
			"error = %v, want unsupported input error",
			err,
		)
	}
}

func TestAgenticToolCallingAdapterPropagatesModelError(
	t *testing.T,
) {
	adapter, err := NewAgenticToolCallingAdapter(
		&adapterAgenticModel{
			err: errors.New("provider unavailable"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = adapter.Generate(
		context.Background(),
		[]*schema.Message{
			schema.UserMessage("plan"),
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"provider unavailable",
		) {
		t.Fatalf(
			"error = %v, want provider error",
			err,
		)
	}
}


func TestAgenticToolCallingAdapterConvertsNormalizedDSMLToolCall(
	t *testing.T,
) {
	base := &adapterAgenticModel{
		response: &schema.AgenticMessage{
			Role: schema.AgenticRoleTypeAssistant,
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(
					&schema.AssistantGenText{
						Text: `<DSML tool_calls><DSML invoke name="plan"><DSML parameter name="steps" string="false">["official"]</DSML parameter></DSML invoke></DSML tool_calls>`,
					},
				),
			},
		},
	}

	normalized := newDSMLNormalizingModel(
		base,
	)
	adapter, err := NewAgenticToolCallingAdapter(
		normalized,
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := adapter.Generate(
		context.Background(),
		[]*schema.Message{
			schema.UserMessage("plan"),
		},
	)
	if err != nil {
		t.Fatalf(
			"Generate() error = %v",
			err,
		)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf(
			"tool calls = %+v",
			result.ToolCalls,
		)
	}
	call := result.ToolCalls[0]
	if call.Function.Name != "plan" ||
		!strings.Contains(
			call.Function.Arguments,
			`"steps":["official"]`,
		) {
		t.Fatalf(
			"normalized tool call = %+v",
			call,
		)
	}
}
