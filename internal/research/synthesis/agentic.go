package synthesis

import (
	"Aegis/internal/agent/event"
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
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"encode research synthesis input: %w",
			err,
		)
	}

	response, err := s.Model.Generate(
		ctx,
		[]*schema.AgenticMessage{
			schema.SystemAgenticMessage(
				researchSynthesisInstruction,
			),
			schema.UserAgenticMessage(
				string(payload),
			),
		},
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"generate research synthesis: %w",
			err,
		)
	}

	raw := strings.TrimSpace(
		assistantText(response),
	)
	if raw == "" {
		return nil, errors.New(
			"research synthesis model returned empty output",
		)
	}

	var synthesized Synthesis
	decoder := json.NewDecoder(
		strings.NewReader(raw),
	)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(
		&synthesized,
	); err != nil {
		return nil, fmt.Errorf(
			"decode research synthesis JSON: %w: output=%q",
			err,
			raw,
		)
	}

	var trailing any
	if err := decoder.Decode(&trailing);
		err != io.EOF {
		if err == nil {
			err = errors.New(
				"multiple JSON values",
			)
		}
		return nil, fmt.Errorf(
			"decode research synthesis JSON: %w: output=%q",
			err,
			raw,
		)
	}

	artifact := &Artifact{
		Symbol:    strings.TrimSpace(input.Symbol),
		Market:    input.Market,
		Technical: input.Technical,
		Event:     input.Event,
		Synthesis: synthesized,
	}
	if err := Validate(artifact); err != nil {
		return nil, fmt.Errorf(
			"invalid research synthesis artifact: %w",
			err,
		)
	}
	return artifact, nil
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

You receive three already-validated typed research contexts:
- market
- technical
- event

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
- Reference domain must be MARKET, TECHNICAL, or EVENT.
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
