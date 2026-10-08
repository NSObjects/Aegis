package planning

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type scriptedResearchModel struct {
	responses []*schema.Message
	err       error
	calls     int
	inputs    [][]*schema.Message
	tools     []*schema.ToolInfo
}

func (m *scriptedResearchModel) Generate(
	_ context.Context,
	messages []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	m.calls++
	m.inputs = append(m.inputs, append([]*schema.Message(nil), messages...))
	if m.err != nil {
		return nil, m.err
	}
	if m.calls > len(m.responses) {
		return nil, errors.New("unexpected extra planning model request")
	}
	return m.responses[m.calls-1], nil
}

func (m *scriptedResearchModel) Stream(
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

func (m *scriptedResearchModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m.tools = append([]*schema.ToolInfo(nil), tools...)
	return m, nil
}

func planningToolMessage(name, args string) *schema.Message {
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID: "call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name: name, Arguments: args,
			},
		}},
	}
}

const goodResearchPlan = `{"steps":[{"id":"official","need":"official_disclosure","scope":"direct","reason":"verify issuer disclosure"}]}`

func TestPlanningToolModelValidPlanSingleCall(t *testing.T) {
	base := &scriptedResearchModel{responses: []*schema.Message{
		planningToolMessage("research_plan", goodResearchPlan),
	}}
	guard := &validatedResearchToolModel{base: base, phase: "planner"}
	_, err := guard.Generate(context.Background(), []*schema.Message{schema.UserMessage("research task")})
	if err != nil || base.calls != 1 {
		t.Fatalf("result: calls=%d error=%v", base.calls, err)
	}
}

func TestPlanningToolModelRetriesEmptyPlanWithActualModelGeneratedPlan(t *testing.T) {
	base := &scriptedResearchModel{responses: []*schema.Message{
		planningToolMessage("research_plan", `{"steps":[]}`),
		planningToolMessage("research_plan", goodResearchPlan),
	}}
	guard := &validatedResearchToolModel{base: base, phase: "planner"}
	original := []*schema.Message{schema.UserMessage("research primary disclosure and relevant news")}
	got, err := guard.Generate(context.Background(), original)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Function.Arguments != goodResearchPlan {
		t.Fatalf("model-generated valid plan was not preserved: %+v", got)
	}
	if base.calls != 2 || len(base.inputs[0]) != 1 || len(base.inputs[1]) != 2 ||
		!strings.Contains(base.inputs[1][1].Content, "requires at least one step") {
		t.Fatalf("retry failed to feed actual plan schema error: %+v", base.inputs)
	}
	if len(original) != 1 {
		t.Fatal("caller input mutated")
	}
}

func TestReplannerEmptyPlanCanBeCorrectedToRespond(t *testing.T) {
	base := &scriptedResearchModel{responses: []*schema.Message{
		planningToolMessage("research_plan", `{"steps":[]}`),
		planningToolMessage("respond", `{"response":"All required evidence acquired, no further gaps."}`),
	}}
	guard := &validatedResearchToolModel{base: base, phase: "replanner"}
	got, err := guard.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("round completed; requirements met"),
	})
	if err != nil || base.calls != 2 || got.ToolCalls[0].Function.Name != "respond" {
		t.Fatalf("replanner must choose completion itself: calls=%d response=%+v error=%v", base.calls, got, err)
	}
}

func TestPlanningToolModelRejectsStillEmptyPlanAfterRetry(t *testing.T) {
	base := &scriptedResearchModel{responses: []*schema.Message{
		planningToolMessage("research_plan", `{"steps":[]}`),
		planningToolMessage("research_plan", `{"steps":[]}`),
	}}
	guard := &validatedResearchToolModel{base: base, phase: "replanner"}
	got, err := guard.Generate(context.Background(), []*schema.Message{schema.UserMessage("need research")})
	if err == nil || !strings.Contains(err.Error(), "replanner returned invalid planning action") ||
		!strings.Contains(err.Error(), "requires at least one step") || got != nil || base.calls != 2 {
		t.Fatalf("invalid plans must fail closed: calls=%d result=%+v err=%v", base.calls, got, err)
	}
}

func TestPlanningToolModelNeverTreatsNoStepsAsSuccess(t *testing.T) {
	for _, tc := range []struct{name, phase, tool, args, want string}{
		{"planner no call", "planner", "", "", "exactly one"},
		{"planner cannot finish", "planner", "respond", `{"response":"done"}`, "planner must produce a research plan"},
		{"replanner empty response", "replanner", "respond", `{"response":""}`, "nonempty"},
		{"replanner unknown tool", "replanner", "unknown", `{"steps":[]}`, "unexpected replanner tool"},
		{"invalid need", "planner", "research_plan", `{"steps":[{"id":"x","need":"unrelated","scope":"direct"}]}`, "unsupported information need"},
	} {
		t.Run(tc.name,func(t *testing.T) {
			var response *schema.Message
			if tc.tool != "" {
				response = planningToolMessage(tc.tool, tc.args)
			} else {
				response = &schema.Message{Role:schema.Assistant}
			}
			base := &scriptedResearchModel{responses: []*schema.Message{response,response}}
			guard := &validatedResearchToolModel{base:base,phase:tc.phase}
			got,err:=guard.Generate(context.Background(),[]*schema.Message{schema.UserMessage("research")})
			if err==nil || !strings.Contains(err.Error(),tc.want) || got!=nil || base.calls!=2 {
				t.Fatalf("calls=%d, result=%+v, err=%v",base.calls,got,err)
			}
		})
	}
}

func TestPlanningToolModelDoesNotRetryProviderFailures(t *testing.T) {
	base:=&scriptedResearchModel{err:errors.New("provider unavailable")}
	guard:=&validatedResearchToolModel{base:base,phase:"planner"}
	got,err:=guard.Generate(context.Background(),[]*schema.Message{schema.UserMessage("research")})
	if err==nil || !strings.Contains(err.Error(),"provider unavailable") || got!=nil || base.calls!=1 {
		t.Fatalf("calls=%d result=%+v error=%v",base.calls,got,err)
	}
}

func TestPlanningToolModelStreamIsGatedAndToolsForwarded(t *testing.T) {
	base:=&scriptedResearchModel{responses:[]*schema.Message{
		planningToolMessage("research_plan", `{"steps":[]}`),
		planningToolMessage("research_plan", goodResearchPlan),
	}}
	raw:=&validatedResearchToolModel{base:base,phase:"planner"}
	withTools,err:=raw.WithTools([]*schema.ToolInfo{&ResearchPlanToolInfo})
	if err!=nil {t.Fatal(err)}
	if len(base.tools)!=1 || base.tools[0].Name!="research_plan" {t.Fatal("planning tool was not forwarded")}
	stream,err:=withTools.Stream(context.Background(),[]*schema.Message{schema.UserMessage("research")})
	if err!=nil {t.Fatal(err)}
	defer stream.Close()
	got,err:=stream.Recv()
	if err!=nil || got==nil || got.ToolCalls[0].Function.Arguments!=goodResearchPlan {
		t.Fatalf("stream returned unvalidated plan: response=%+v, err=%v",got,err)
	}
	if base.calls!=2 {t.Fatalf("stream calls=%d, want 2",base.calls)}
}
