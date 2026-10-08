package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Type    string                  `json:"type,omitempty"`
	Matches []SemanticEvidenceMatch `json:"matches"`
}

const semanticEvidenceBatchSize = 4

// Match evaluates a bounded set of candidate documents at a time so that
// output length and response completeness do not scale with the entire news
// feed. Missing IDs are retried individually. If any remain unclassified,
// the caller gets an explicit incomplete-research error, never a partial
// set presented as complete.
func (m *AgenticSemanticEvidenceMatcher) Match(
	ctx context.Context,
	target EvidenceTarget,
	documents []RawDocument,
) ([]SemanticEvidenceMatch, error) {
	if m == nil || m.Model == nil {
		return nil, errors.New("semantic evidence matcher model is required")
	}
	matches := make([]SemanticEvidenceMatch, 0, len(documents))
	for start := 0; start < len(documents); start += semanticEvidenceBatchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := start + semanticEvidenceBatchSize
		if end > len(documents) {
			end = len(documents)
		}
		batch := documents[start:end]
		first, err := m.matchBatch(ctx, target, batch)
		if err != nil {
			return nil, fmt.Errorf("semantic evidence batch %d: %w",
				start/semanticEvidenceBatchSize+1, err)
		}
		// Reject fabricated IDs, duplicate IDs or invalid confidence BEFORE
		// accepting any subset from this model response.
		if _, err := validatePartialSemanticMatches(batch, first, defaultSemanticSelectionThreshold); err != nil {
			return nil, fmt.Errorf("validate semantic evidence batch %d: %w",
				start/semanticEvidenceBatchSize+1, err)
		}
		seen := make(map[string]struct{}, len(first))
		for _, match := range first {
			seen[strings.TrimSpace(match.RawDocumentID)] = struct{}{}
		}
		omitted := make([]RawDocument, 0)
		for _, document := range batch {
			if _, ok := seen[strings.TrimSpace(document.ID)]; !ok {
				omitted = append(omitted, document)
			}
		}
		// Only re-evaluate missing IDs, one document per request. A small
		// batch can still omit an ID due to prompt and output drift; the
		// single-document retry bounds output size without weakening coverage.
		for _, document := range omitted {
			recovered, err := m.matchBatch(ctx, target, []RawDocument{document})
			if err != nil {
				return nil, fmt.Errorf("retry semantic document %q: %w",
					document.ID, err)
			}
			if _, err := validatePartialSemanticMatches(
				[]RawDocument{document}, recovered,
				defaultSemanticSelectionThreshold,
			); err != nil {
				return nil, fmt.Errorf("validate retried semantic document %q: %w",
					document.ID, err)
			}
			first = append(first, recovered...)
		}
		if _, err := validateSemanticMatches(
			batch, first, defaultSemanticSelectionThreshold,
		); err != nil {
			return nil, fmt.Errorf("semantic evidence batch %d: %w",
				start/semanticEvidenceBatchSize+1, err)
		}
		matches = append(matches, first...)
	}
	return matches, nil
}

func (m *AgenticSemanticEvidenceMatcher) matchBatch(
	ctx context.Context,
	target EvidenceTarget,
	documents []RawDocument,
) ([]SemanticEvidenceMatch, error) {
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
	decoder := json.NewDecoder(
		strings.NewReader(raw),
	)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf(
			"invalid semantic evidence matcher JSON: %w: output=%q",
			err,
			raw,
		)
	}
	if parsed.Type != "" &&
		parsed.Type != "json_object" {
		return nil, fmt.Errorf(
			"invalid semantic evidence matcher JSON: unsupported type %q",
			parsed.Type,
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
