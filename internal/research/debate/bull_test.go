package debate

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type bullModelStub struct{ output string }

func (m *bullModelStub) Generate(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.AgenticMessage, error) {
	return &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: m.output})}}, nil
}
func (m *bullModelStub) Stream(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	panic("unexpected Stream call")
}

func TestBullResearcherGeneratesGroundedOpening(t *testing.T) {
	r := &BullResearcher{Model: &bullModelStub{output: `{
		"round":1,"side":"BULL","thesis":"constructive setup",
		"arguments":[{"id":"bull-r1-a1","claim":"technical context supports the constructive case","references":[{"domain":"TECHNICAL"}]}],
		"confidence":0.7
	}`}}
	turn, err := r.GenerateTurn(context.Background(), validArtifact(), &State{Symbol: "600519.SH"}, 1)
	if err != nil { t.Fatalf("GenerateTurn() error = %v", err) }
	if turn.Side != SideBull || turn.Round != 1 { t.Fatalf("turn = %#v", turn) }
}

func TestBullResearcherRejectsInventedEventReference(t *testing.T) {
	r := &BullResearcher{Model: &bullModelStub{output: `{
		"round":1,"side":"BULL","thesis":"unsupported",
		"arguments":[{"id":"bull-r1-a1","claim":"invented event","references":[{"domain":"EVENT","event_id":"invented"}]}],
		"confidence":0.7
	}`}}
	_, err := r.GenerateTurn(context.Background(), validArtifact(), &State{Symbol: "600519.SH"}, 1)
	if err == nil || !strings.Contains(err.Error(), "does not exist") { t.Fatalf("error = %v", err) }
}

func TestBullResearcherRejectsTradingActionAsSide(t *testing.T) {
	r := &BullResearcher{Model: &bullModelStub{output: `{
		"round":1,"side":"BEAR","thesis":"wrong role",
		"arguments":[{"id":"x","claim":"claim","references":[{"domain":"MARKET"}]}],
		"confidence":0.5
	}`}}
	_, err := r.GenerateTurn(context.Background(), validArtifact(), &State{Symbol: "600519.SH"}, 1)
	if err == nil || !strings.Contains(err.Error(), "returned side") { t.Fatalf("error = %v", err) }
}
