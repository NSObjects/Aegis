package debate

import (
	"Aegis/internal/research/synthesis"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/model"
)

type BearResearcher struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

type bearInput struct {
	Artifact *synthesis.Artifact `json:"research_artifact"`
	Debate   *State              `json:"debate_state"`
	Round    int                 `json:"round"`
}

func (r *BearResearcher) GenerateTurn(ctx context.Context, artifact *synthesis.Artifact, state *State, round int) (Turn, error) {
	if r == nil || r.Model == nil { return Turn{}, errors.New("bear researcher model is required") }
	if err := synthesis.Validate(artifact); err != nil { return Turn{}, fmt.Errorf("invalid research artifact: %w", err) }
	if state == nil { state = &State{Symbol: artifact.Symbol} }
	if err := ValidateState(state, artifact); err != nil { return Turn{}, fmt.Errorf("invalid debate state: %w", err) }
	if round <= 0 { return Turn{}, errors.New("bear debate round must be positive") }

	payload, err := json.Marshal(bearInput{Artifact: artifact, Debate: state, Round: round})
	if err != nil { return Turn{}, fmt.Errorf("encode bear researcher input: %w", err) }
	return generateValidatedTurn(
		ctx, r.Model, bearInstruction, string(payload),
		artifact, state, round, SideBear,
	)
}

const bearInstruction = `You are the Bear Researcher in Aegis' investment research debate.

Your role follows the Bear Researcher in TradingAgents: construct the strongest evidence-based downside thesis and, when prior bullish arguments exist, rebut them directly.

Hard constraints:
- Use ONLY the supplied validated research_artifact and debate_state.
- Do not use outside knowledge and do not request or call tools.
- Do not invent facts, evidence IDs, event IDs, prices, forecasts, or research.
- Do not output BUY, SELL, WATCH, AVOID, CANDIDATE, position size, or price targets.
- Each argument MUST contain a nonempty unique id (including across all previous debate_state turns), a nonempty claim, and one or more valid references.
- Every argument and rebuttal must cite one or more supplied ResearchArtifact references.
- Reference domains are MARKET, TECHNICAL, EVENT.
- EVENT event_id, when present, must exist in research_artifact.event.events.
- Rebuttals may target only argument IDs already present in debate_state; never target an argument created in the same turn.
- Keep positive evidence and uncertainty visible; advocate the bear case without pretending weak evidence is strong evidence.
- side must be BEAR.
- round must exactly equal the requested round.
- confidence must be between 0 and 1.
- Return one JSON object only, with no markdown.

Schema:
{
  "round": 1,
  "side": "BEAR",
  "thesis": "string",
  "arguments": [
    {
      "id": "unique stable id",
      "claim": "string",
      "references": [{"domain":"MARKET"}]
    }
  ],
  "rebuttals": [
    {
      "claim": "string",
      "target_argument_id": "existing prior argument id",
      "references": [{"domain":"TECHNICAL"}]
    }
  ],
  "confidence": 0.0
}`
