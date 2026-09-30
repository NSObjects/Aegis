package debate

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"Aegis/internal/research/synthesis"
)

type bearModelStub struct{ output string }

func (m *bearModelStub) Generate(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.AgenticMessage, error) {
	return &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: m.output})}}, nil
}
func (m *bearModelStub) Stream(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	panic("unexpected Stream call")
}

func TestBearResearcherGeneratesGroundedRebuttal(t *testing.T) {
	state := &State{Symbol: "600519.SH", Turns: []Turn{{
		Round: 1, Side: SideBull, Thesis: "bull thesis",
		Arguments: []Argument{{ID: "bull-r1-a1", Claim: "constructive technical setup", References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}}}},
		Confidence: .7,
	}}}
	r := &BearResearcher{Model: &bearModelStub{output: `{
		"round":1,"side":"BEAR","thesis":"downside remains material",
		"arguments":[{"id":"bear-r1-a1","claim":"market context remains a risk","references":[{"domain":"MARKET"}]}],
		"rebuttals":[{"claim":"the technical case does not remove event uncertainty","target_argument_id":"bull-r1-a1","references":[{"domain":"EVENT","event_id":"event-1"}]}],
		"confidence":0.68
	}`}}
	turn, err := r.GenerateTurn(context.Background(), validArtifact(), state, 1)
	if err != nil { t.Fatalf("GenerateTurn() error = %v", err) }
	if turn.Side != SideBear || len(turn.Rebuttals) != 1 { t.Fatalf("turn = %#v", turn) }
}

func TestBearResearcherRejectsFutureRebuttalTarget(t *testing.T) {
	r := &BearResearcher{Model: &bearModelStub{output: `{
		"round":1,"side":"BEAR","thesis":"downside",
		"arguments":[{"id":"bear-r1-a1","claim":"risk","references":[{"domain":"MARKET"}]}],
		"rebuttals":[{"claim":"invalid target","target_argument_id":"future-bull","references":[{"domain":"MARKET"}]}],
		"confidence":0.6
	}`}}
	_, err := r.GenerateTurn(context.Background(), validArtifact(), &State{Symbol: "600519.SH"}, 1)
	if err == nil || !strings.Contains(err.Error(), "prior argument") { t.Fatalf("error = %v", err) }
}

func TestBearResearcherRejectsWrongSide(t *testing.T) {
	r := &BearResearcher{Model: &bearModelStub{output: `{
		"round":1,"side":"BULL","thesis":"wrong",
		"arguments":[{"id":"x","claim":"claim","references":[{"domain":"MARKET"}]}],
		"confidence":0.5
	}`}}
	_, err := r.GenerateTurn(context.Background(), validArtifact(), &State{Symbol: "600519.SH"}, 1)
	if err == nil || !strings.Contains(err.Error(), "returned side") { t.Fatalf("error = %v", err) }
}
