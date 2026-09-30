package debate

import (
	"Aegis/internal/research/synthesis"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type BullResearcher struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

type bullInput struct {
	Artifact *synthesis.Artifact `json:"research_artifact"`
	Debate   *State              `json:"debate_state"`
	Round    int                 `json:"round"`
}

func (r *BullResearcher) GenerateTurn(ctx context.Context, artifact *synthesis.Artifact, state *State, round int, opts ...model.Option) (Turn, error) {
	if r == nil || r.Model == nil { return Turn{}, errors.New("bull researcher model is required") }
	if err := synthesis.Validate(artifact); err != nil { return Turn{}, fmt.Errorf("invalid research artifact: %w", err) }
	if state == nil { state = &State{Symbol: artifact.Symbol} }
	if err := ValidateState(state, artifact); err != nil { return Turn{}, fmt.Errorf("invalid debate state: %w", err) }
	if round <= 0 { return Turn{}, errors.New("bull debate round must be positive") }

	payload, err := json.Marshal(bullInput{Artifact: artifact, Debate: state, Round: round})
	if err != nil { return Turn{}, fmt.Errorf("encode bull researcher input: %w", err) }
	response, err := r.Model.Generate(ctx, []*schema.AgenticMessage{
		schema.SystemAgenticMessage(bullInstruction),
		schema.UserAgenticMessage(string(payload)),
	}, opts...)
	if err != nil { return Turn{}, fmt.Errorf("generate bull researcher turn: %w", err) }
	raw := strings.TrimSpace(debateAssistantText(response))
	if raw == "" { return Turn{}, errors.New("bull researcher returned empty output") }
	turn, err := decodeTurn(raw)
	if err != nil { return Turn{}, err }
	if turn.Side != SideBull { return Turn{}, fmt.Errorf("bull researcher returned side %q", turn.Side) }
	if turn.Round != round { return Turn{}, fmt.Errorf("bull researcher returned round %d, want %d", turn.Round, round) }

	candidate := &State{Symbol: state.Symbol, Turns: append(append([]Turn(nil), state.Turns...), turn)}
	if err := ValidateState(candidate, artifact); err != nil { return Turn{}, fmt.Errorf("invalid bull researcher turn: %w", err) }
	return turn, nil
}

func decodeTurn(raw string) (Turn, error) {
	var turn Turn
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&turn); err != nil { return Turn{}, fmt.Errorf("decode debate turn JSON: %w: output=%q", err, raw) }
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil { err = errors.New("multiple JSON values") }
		return Turn{}, fmt.Errorf("decode debate turn JSON: %w: output=%q", err, raw)
	}
	return turn, nil
}

func debateAssistantText(message *schema.AgenticMessage) string {
	if message == nil || message.Role != schema.AgenticRoleTypeAssistant { return "" }
	var b strings.Builder
	for _, block := range message.ContentBlocks {
		if block != nil && block.Type == schema.ContentBlockTypeAssistantGenText && block.AssistantGenText != nil {
			b.WriteString(block.AssistantGenText.Text)
		}
	}
	return b.String()
}

const bullInstruction = `You are the Bull Researcher in Aegis' investment research debate.

Your role follows the Bull Researcher in TradingAgents: construct the strongest evidence-based bullish thesis and, when prior bearish arguments exist, rebut them directly.

Hard constraints:
- Use ONLY the supplied validated research_artifact and debate_state.
- Do not use outside knowledge and do not request or call tools.
- Do not invent facts, evidence IDs, event IDs, prices, forecasts, or research.
- Do not output BUY, SELL, WATCH, AVOID, CANDIDATE, position size, or price targets.
- Every argument and rebuttal must cite one or more supplied ResearchArtifact references.
- Reference domains are MARKET, TECHNICAL, EVENT.
- EVENT event_id, when present, must exist in research_artifact.event.events.
- Rebuttals may target only argument IDs already present in debate_state; never target an argument created in the same turn.
- Keep uncertainty and upstream risk visible; advocate the bull case without pretending weak evidence is strong evidence.
- side must be BULL.
- round must exactly equal the requested round.
- confidence must be between 0 and 1.
- Return one JSON object only, with no markdown.

Schema:
{
  "round": 1,
  "side": "BULL",
  "thesis": "string",
  "arguments": [
    {
      "id": "unique stable id",
      "claim": "string",
      "references": [{"domain":"TECHNICAL"}]
    }
  ],
  "rebuttals": [
    {
      "claim": "string",
      "target_argument_id": "existing prior argument id",
      "references": [{"domain":"EVENT","event_id":"existing_event_id"}]
    }
  ],
  "confidence": 0.0
}`
