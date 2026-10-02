package manager

import (
	"Aegis/internal/research/debate"
	"Aegis/internal/research/synthesis"
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type managerModelStub struct{ output string }

func (m *managerModelStub) Generate(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.AgenticMessage, error) {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: m.output}),
		},
	}, nil
}

func (m *managerModelStub) Stream(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	panic("unexpected Stream call")
}

func managerArtifact() *synthesis.Artifact {
	return validManagerArtifact()
}

func managerDebate() *debate.State {
	return &debate.State{
		Symbol: "600519.SH",
		Turns: []debate.Turn{
			{
				Round: 1,
				Side: debate.SideBull,
				Thesis: "constructive",
				Arguments: []debate.Argument{
					{ID: "bull-r1-a1", Claim: "bull claim", References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}}},
				},
				Confidence: .7,
			},
			{
				Round: 1,
				Side: debate.SideBear,
				Thesis: "cautious",
				Arguments: []debate.Argument{
					{ID: "bear-r1-a1", Claim: "bear claim", References: []synthesis.Reference{{Domain: synthesis.DomainMarket}}},
				},
				Confidence: .6,
			},
		},
	}
}

func TestResearchManagerBuildsGroundedPlan(t *testing.T) {
	m := &ResearchManager{Model: &managerModelStub{output: `{
		"symbol":"600519.SH",
		"conclusion":"BALANCED",
		"summary":"bullish technical evidence and bearish market evidence remain in tension",
		"opportunities":[{"claim":"technical setup remains constructive","references":[{"domain":"TECHNICAL"}]}],
		"risks":[{"claim":"market context remains a headwind","references":[{"domain":"MARKET"}]}],
		"accepted_argument_ids":["bull-r1-a1"],
		"unresolved_argument_ids":["bear-r1-a1"],
		"trading_constraints":["treat the unresolved market headwind as material uncertainty"],
		"confidence":0.65
	}`}}
	plan, err := m.BuildPlan(context.Background(), managerArtifact(), managerDebate())
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.Conclusion != ConclusionBalanced {
		t.Fatalf("conclusion = %q", plan.Conclusion)
	}
}

func TestResearchManagerRejectsUnknownArgumentID(t *testing.T) {
	m := &ResearchManager{Model: &managerModelStub{output: `{
		"symbol":"600519.SH",
		"conclusion":"BULL_LEAN",
		"summary":"constructive",
		"opportunities":[{"claim":"technical setup remains constructive","references":[{"domain":"TECHNICAL"}]}],
		"accepted_argument_ids":["invented"],
		"confidence":0.7
	}`}}
	_, err := m.BuildPlan(context.Background(), managerArtifact(), managerDebate())
	if err == nil || !strings.Contains(err.Error(), "does not exist in debate") {
		t.Fatalf("error = %v", err)
	}
}

func TestResearchManagerRejectsInventedEventReference(t *testing.T) {
	m := &ResearchManager{Model: &managerModelStub{output: `{
		"symbol":"600519.SH",
		"conclusion":"BEAR_LEAN",
		"summary":"cautious",
		"risks":[{"claim":"invented event risk","references":[{"domain":"EVENT","event_id":"invented"}]}],
		"confidence":0.7
	}`}}
	_, err := m.BuildPlan(context.Background(), managerArtifact(), managerDebate())
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v", err)
	}
}

func TestResearchManagerRequiresDebate(t *testing.T) {
	m := &ResearchManager{Model: &managerModelStub{}}
	_, err := m.BuildPlan(context.Background(), managerArtifact(), &debate.State{Symbol: "600519.SH"})
	if err == nil || !strings.Contains(err.Error(), "non-empty debate") {
		t.Fatalf("error = %v", err)
	}
}
