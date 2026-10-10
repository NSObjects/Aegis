package synthesis

import (
	"Aegis/internal/agent/event"
	"Aegis/internal/agent/fundamental"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type AgenticSynthesizer struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

type modelInput struct {
	Symbol    string             `json:"symbol"`
	Market    *market.Context    `json:"market"`
	Technical *technical.Context `json:"technical"`
	Event     *event.Context     `json:"event"`
	Fundamental *fundamental.Context `json:"fundamental,omitempty"`
}

func (s *AgenticSynthesizer) Synthesize(
	ctx context.Context,
	input Input,
	opts ...model.Option,
) (*Artifact, error) {
	if s == nil || s.Model == nil {
		return nil, errors.New(
			"research synthesis model is required",
		)
	}
	if err := ValidateInput(input); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(
		modelInput{
			Symbol:    strings.TrimSpace(input.Symbol),
			Market:    input.Market,
			Technical: input.Technical,
			Event:     input.Event,
			Fundamental: input.Fundamental,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"encode research synthesis input: %w",
			err,
		)
	}

	messages := []*schema.AgenticMessage{
		schema.SystemAgenticMessage(researchSynthesisInstruction),
		schema.UserAgenticMessage(string(payload)),
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		response, err := s.Model.Generate(ctx, messages, opts...)
		if err != nil {
			return nil, fmt.Errorf("generate research synthesis: %w", err)
		}
		raw := strings.TrimSpace(assistantText(response))
		if raw == "" {
			return nil, errors.New("research synthesis model returned empty output")
		}
		synthesized, err := decodeSynthesis(raw)
		if err == nil {
			artifact := &Artifact{
				Symbol: strings.TrimSpace(input.Symbol),
				Market: input.Market,
				Technical: input.Technical,
				Event: input.Event,
				Fundamental: input.Fundamental,
				Synthesis: synthesized,
			}
			if validationErr := Validate(artifact); validationErr == nil {
				return artifact, nil
			} else {
				err = fmt.Errorf("invalid research synthesis artifact: %w", validationErr)
			}
		}
		lastErr = err
		if attempt == 0 {
			messages = append(messages,
				response,
				schema.UserAgenticMessage(fmt.Sprintf(
					"Your previous JSON failed deterministic validation: %v. Return the complete corrected JSON object only. Preserve upstream facts and obey the original schema; in particular, every conflict must reference at least two genuinely supported distinct domains. If a claimed conflict is supported within only one domain, REMOVE that conflict and, if necessary, describe its uncertainty under risks. Do not attach an unrelated domain citation to satisfy validation. Preserve upstream facts unchanged.",
					err,
				)),
			)
		}
	}
	return nil, lastErr
}

func decodeSynthesis(raw string) (Synthesis, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil {
		if kind, ok := envelope["type"]; ok {
			var value string
			if err := json.Unmarshal(kind, &value); err != nil || value != "json_object" {
				return Synthesis{}, fmt.Errorf("decode research synthesis JSON: invalid type metadata: output=%q", raw)
			}
			delete(envelope, "type")
			normalized, err := json.Marshal(envelope)
			if err != nil { return Synthesis{}, fmt.Errorf("normalize research synthesis JSON: %w", err) }
			raw = string(normalized)
		}
	}
	var synthesized Synthesis
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&synthesized); err != nil {
		return Synthesis{}, fmt.Errorf("decode research synthesis JSON: %w: output=%q", err, raw)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil { err = errors.New("multiple JSON values") }
		return Synthesis{}, fmt.Errorf("decode research synthesis JSON: %w: output=%q", err, raw)
	}
	return synthesized, nil
}

func assistantText(
	message *schema.AgenticMessage,
) string {
	if message == nil ||
		message.Role !=
			schema.AgenticRoleTypeAssistant {
		return ""
	}

	var builder strings.Builder
	for _, block := range message.ContentBlocks {
		if block == nil ||
			block.Type !=
				schema.ContentBlockTypeAssistantGenText ||
			block.AssistantGenText == nil {
			continue
		}
		builder.WriteString(
			block.AssistantGenText.Text,
		)
	}
	return builder.String()
}

const researchSynthesisInstruction = `You are the cross-domain research synthesis layer for Aegis.

You receive three required already-validated research contexts and an optional fourth:
- market
- technical
- event
- fundamental (optional, source-owned announced financial metrics plus interpretation)

Your role is to identify cross-domain agreement, disagreement, uncertainty, and research risk.

Rules:
- Return JSON only.
- Do not create, remove, or rewrite upstream facts.
- Do not invent new event facts or outside information.
- Do not output trading actions such as BUY, SELL, WATCH, AVOID, or CANDIDATE.
- Do not output price targets or position-sizing instructions.
- Do not pretend the different upstream time fields have identical precision.
- Preserve uncertainty from NO_DATA, SUSPENDED, NO_EVENT, or INSUFFICIENT_EVIDENCE states.
- Every agreement must cite at least one supplied domain.
- Every conflict must cite at least two different supplied domains.
- Reference domain must be MARKET, TECHNICAL, EVENT, or FUNDAMENTAL (only if fundamental context is present). FUNDAMENTAL must not carry event_id.
- event_id may be supplied only for EVENT references and must come from the provided EventContext.
- confidence must be between 0 and 1.

Output schema:
{
  "summary": "string",
  "agreements": [
    {
      "claim": "string",
      "references": [
        {"domain":"MARKET"},
        {"domain":"EVENT","event_id":"event_x"}
      ]
    }
  ],
  "conflicts": [
    {
      "description": "string",
      "references": [
        {"domain":"TECHNICAL"},
        {"domain":"EVENT","event_id":"event_x"}
      ]
    }
  ],
  "risks": ["string"],
  "confidence": 0.0
}`
