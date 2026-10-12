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
				// An LLM can continue inventing "cross-domain" conflicts backed
				// by a single domain even after a corrective prompt. Only after
				// the retry is exhausted may we omit those unsupported *relations*.
				// Every retained claim, reference and the whole artifact must
				// still pass unchanged deterministic validation.
				if attempt == 1 {
					filtered, count := omitSingleDomainConflictCandidates(artifact)
					if count > 0 && Validate(filtered) == nil {
						return filtered, nil
					}
				}
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


const omittedConflictRisk = "Generated cross-domain conflict candidate(s) omitted for insufficient independent domain references; omitted claims were not verified or resolved."

// omitSingleDomainConflictCandidates is a conservative last-resort fallback:
// it removes only *purported cross-domain relations* whose references are
// individually valid but all belong to the same domain. This does not infer
// new references, change upstream facts or normalize any other malformed data.
// The caller must validate the entire result again before accepting it.
func omitSingleDomainConflictCandidates(artifact *Artifact) (*Artifact, int) {
	if artifact == nil || artifact.Event == nil {
		return artifact, 0
	}
	eventIDs := make(map[string]struct{}, len(artifact.Event.Events))
	for _, e := range artifact.Event.Events {
		eventIDs[e.ID] = struct{}{}
	}
	kept := make([]Conflict, 0, len(artifact.Synthesis.Conflicts))
	removed := 0
	for _, conflict := range artifact.Synthesis.Conflicts {
		if strings.TrimSpace(conflict.Description) == "" || len(conflict.References) < 2 {
			kept = append(kept, conflict)
			continue
		}
		domains := make(map[Domain]struct{})
		valid := true
		for _, ref := range conflict.References {
			if err := validateReference(ref, eventIDs, artifact.Fundamental != nil); err != nil {
				valid = false
				break
			}
			domains[ref.Domain] = struct{}{}
		}
		if valid && len(domains) == 1 {
			removed++
			continue
		}
		kept = append(kept, conflict)
	}
	if removed == 0 {
		return artifact, 0
	}
	copy := *artifact
	copy.Synthesis = artifact.Synthesis
	copy.Synthesis.Conflicts = kept
	copy.Synthesis.Risks = append(append([]string(nil), artifact.Synthesis.Risks...), omittedConflictRisk)
	return &copy, removed
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

const researchSynthesisInstruction = `
【输出语言强制要求】所有面向人阅读的分析、结论、观点、理由、风险、约束和解释必须使用简体中文，不要整段使用英文。仅 JSON 字段名、固定枚举值、证券代码、模型/技术缩写、事件/论点/文档 ID、公司正式名称和准确的外文专有名词可保留原文。不得因翻译改变已有事实、引用 ID、数值、时间、来源或证据内容。
You are the cross-domain research synthesis layer for Aegis.

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
