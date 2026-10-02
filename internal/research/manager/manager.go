package manager

import (
	"Aegis/internal/research/debate"
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

type ResearchManager struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

type managerInput struct {
	Artifact *synthesis.Artifact `json:"research_artifact"`
	Debate   *debate.State       `json:"debate_state"`
}

func (m *ResearchManager) BuildPlan(ctx context.Context, artifact *synthesis.Artifact, state *debate.State, opts ...model.Option) (*ResearchPlan, error) {
	if m == nil || m.Model == nil {
		return nil, errors.New("research manager model is required")
	}
	if err := synthesis.Validate(artifact); err != nil {
		return nil, fmt.Errorf("invalid research artifact: %w", err)
	}
	if err := debate.ValidateState(state, artifact); err != nil {
		return nil, fmt.Errorf("invalid debate state: %w", err)
	}
	if len(state.Turns) == 0 {
		return nil, errors.New("research manager requires a non-empty debate")
	}

	payload, err := json.Marshal(managerInput{Artifact: artifact, Debate: state})
	if err != nil {
		return nil, fmt.Errorf("encode research manager input: %w", err)
	}
	response, err := m.Model.Generate(ctx, []*schema.AgenticMessage{
		schema.SystemAgenticMessage(researchManagerInstruction),
		schema.UserAgenticMessage(string(payload)),
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("generate research plan: %w", err)
	}
	raw := strings.TrimSpace(managerAssistantText(response))
	if raw == "" {
		return nil, errors.New("research manager returned empty output")
	}
	plan, err := decodeResearchPlan(raw)
	if err != nil {
		return nil, err
	}
	if err := Validate(&plan, artifact, state); err != nil {
		return nil, fmt.Errorf("invalid research plan: %w", err)
	}
	return &plan, nil
}

func decodeResearchPlan(raw string) (ResearchPlan, error) {
	var plan ResearchPlan
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return ResearchPlan{}, fmt.Errorf("decode research plan JSON: %w: output=%q", err, raw)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return ResearchPlan{}, fmt.Errorf("decode research plan JSON: %w: output=%q", err, raw)
	}
	return plan, nil
}

func managerAssistantText(message *schema.AgenticMessage) string {
	if message == nil || message.Role != schema.AgenticRoleTypeAssistant {
		return ""
	}
	var b strings.Builder
	for _, block := range message.ContentBlocks {
		if block != nil && block.Type == schema.ContentBlockTypeAssistantGenText && block.AssistantGenText != nil {
			b.WriteString(block.AssistantGenText.Text)
		}
	}
	return b.String()
}

const researchManagerInstruction = `You are the Research Manager in Aegis.

You receive:
- one validated ResearchArtifact
- one validated completed Bull/Bear DebateState

Your job is to judge the research debate and produce a structured ResearchPlan for the downstream Trading Agent.

Hard constraints:
- Use ONLY the supplied research_artifact and debate_state.
- Do not call tools or perform new research.
- Do not invent facts, event IDs, argument IDs, prices, forecasts, or evidence.
- Do not output BUY, SELL, WATCH, AVOID, CANDIDATE, position size, or price targets.
- This is a research judgment layer, not the final trading decision layer.
- Every opportunity and risk must cite existing ResearchArtifact references.
- accepted_argument_ids and unresolved_argument_ids may reference only argument IDs already present in debate_state.
- An argument ID may not appear in both accepted_argument_ids and unresolved_argument_ids.
- trading_constraints are qualitative constraints for the downstream Trading Agent; they must not contain final actions, sizing, or price targets.
- Preserve uncertainty and conflicts instead of forcing false certainty.
- confidence must be between 0 and 1.
- Return one JSON object only, with no markdown.

Conclusion meanings:
- BULL_LEAN: the validated debate leaves the research case tilted bullish.
- BEAR_LEAN: the validated debate leaves the research case tilted bearish.
- BALANCED: meaningful bullish and bearish evidence remain in tension.
- INSUFFICIENT_EVIDENCE: the supplied evidence/debate is not strong enough for a directional research lean.

Schema:
{
  "symbol":"600519.SH",
  "conclusion":"BULL_LEAN|BEAR_LEAN|BALANCED|INSUFFICIENT_EVIDENCE",
  "summary":"string",
  "opportunities":[
    {"claim":"string","references":[{"domain":"TECHNICAL"}]}
  ],
  "risks":[
    {"claim":"string","references":[{"domain":"EVENT","event_id":"existing_event_id"}]}
  ],
  "accepted_argument_ids":["existing_argument_id"],
  "unresolved_argument_ids":["existing_argument_id"],
  "trading_constraints":["qualitative constraint"],
  "confidence":0.0
}`
