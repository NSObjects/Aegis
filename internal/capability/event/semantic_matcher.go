package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type AgenticSemanticEvidenceMatcher struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

var _ SemanticEvidenceMatcher = (*AgenticSemanticEvidenceMatcher)(nil)

type semanticEvidencePrompt struct {
	Target    EvidenceTarget             `json:"target"`
	Documents []semanticEvidenceDocument `json:"documents"`
}

type semanticEvidenceDocument struct {
	RawDocumentID string `json:"raw_document_id"`
	Title         string `json:"title,omitempty"`
	Content       string `json:"content,omitempty"`
	Publisher     string `json:"publisher,omitempty"`
}

type semanticEvidenceResponse struct {
	Matches []SemanticEvidenceMatch `json:"matches"`
}

func (m *AgenticSemanticEvidenceMatcher) Match(
	ctx context.Context,
	target EvidenceTarget,
	documents []RawDocument,
) ([]SemanticEvidenceMatch, error) {
	if m == nil || m.Model == nil {
		return nil, errors.New(
			"semantic evidence matcher model is required",
		)
	}

	prompt := semanticEvidencePrompt{
		Target: target,
		Documents: make(
			[]semanticEvidenceDocument,
			0,
			len(documents),
		),
	}
	for _, document := range documents {
		prompt.Documents = append(
			prompt.Documents,
			semanticEvidenceDocument{
				RawDocumentID: document.ID,
				Title:         document.Title,
				Content:       document.Content,
				Publisher:     document.Publisher,
			},
		)
	}

	payload, err := json.Marshal(prompt)
	if err != nil {
		return nil, fmt.Errorf(
			"encode semantic evidence prompt: %w",
			err,
		)
	}

	response, err := m.Model.Generate(
		ctx,
		[]*schema.AgenticMessage{
			agenticTextMessage(
				schema.AgenticRoleTypeSystem,
				semanticEvidenceInstruction,
			),
			agenticTextMessage(
				schema.AgenticRoleTypeUser,
				string(payload),
			),
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"generate semantic evidence relevance: %w",
			err,
		)
	}

	raw := strings.TrimSpace(
		agenticAssistantText(response),
	)
	if raw == "" {
		return nil, errors.New(
			"semantic evidence matcher returned empty output",
		)
	}

	var parsed semanticEvidenceResponse
	if err := json.Unmarshal(
		[]byte(raw),
		&parsed,
	); err != nil {
		return nil, fmt.Errorf(
			"invalid semantic evidence matcher JSON: %w: output=%q",
			err,
			raw,
		)
	}

	return parsed.Matches, nil
}

const semanticEvidenceInstruction = `You are a strict evidence relevance classifier.

Determine whether each candidate document is materially about the target security/company or directly describes an event that materially concerns that target.

Rules:
- Return JSON only.
- Return exactly one decision for every input raw_document_id.
- Do not add or change IDs.
- relevant=true only when the document itself supports a target-specific connection.
- Do not infer relevance merely because the document concerns the same sector, theme, macro topic, competitor, supplier, customer, or market unless the target connection is explicit or directly material.
- If the evidence is ambiguous, set relevant=false or use a low confidence.
- confidence must be between 0 and 1.
- reason must be concise and evidence-based.
- Never use information outside the provided target and document contents.

Output schema:
{
  "matches": [
    {
      "raw_document_id": "string",
      "relevant": true,
      "confidence": 0.0,
      "reason": "string"
    }
  ]
}`

func agenticTextMessage(
	role schema.AgenticRoleType,
	text string,
) *schema.AgenticMessage {
	block := &schema.ContentBlock{}
	switch role {
	case schema.AgenticRoleTypeAssistant:
		block.Type =
			schema.ContentBlockTypeAssistantGenText
		block.AssistantGenText =
			&schema.AssistantGenText{
				Text: text,
			}
	default:
		block.Type =
			schema.ContentBlockTypeUserInputText
		block.UserInputText =
			&schema.UserInputText{
				Text: text,
			}
	}

	return &schema.AgenticMessage{
		Role: role,
		ContentBlocks: []*schema.ContentBlock{
			block,
		},
	}
}

func agenticAssistantText(
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
