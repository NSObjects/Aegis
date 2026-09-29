package event

import (
	capevent "Aegis/internal/capability/event"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type AgenticEventFactExtractor struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

var _ EventFactExtractor = (*AgenticEventFactExtractor)(nil)

type AgenticEventClusterer struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

var _ EventClusterer = (*AgenticEventClusterer)(nil)

type AgenticEventMaterialityAnalyzer struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

var _ EventMaterialityAnalyzer = (*AgenticEventMaterialityAnalyzer)(nil)

type factExtractionPrompt struct {
	Symbol    string                   `json:"symbol"`
	AsOf      string                   `json:"as_of"`
	Documents []factExtractionDocument `json:"documents"`
}

type factExtractionDocument struct {
	ID           string                      `json:"id"`
	Title        string                      `json:"title,omitempty"`
	Content      string                      `json:"content,omitempty"`
	Publisher    string                      `json:"publisher,omitempty"`
	SourceType   capevent.SourceType         `json:"source_type,omitempty"`
	Authority    capevent.EvidenceAuthority  `json:"authority,omitempty"`
	EvidenceKind capevent.EvidenceKind       `json:"evidence_kind,omitempty"`
	IsPrimary    bool                        `json:"is_primary"`
	PublishedAt  string                      `json:"published_at,omitempty"`
	UpdatedAt    string                      `json:"updated_at,omitempty"`
	AvailableAt  string                      `json:"available_at,omitempty"`
}

type clusteringPrompt struct {
	Symbol string `json:"symbol"`
	Facts  []Fact `json:"facts"`
}

type materialityPrompt struct {
	Symbol          string                   `json:"symbol"`
	AsOf            string                   `json:"as_of"`
	EventID         string                   `json:"event_id"`
	Type            EventType                `json:"type"`
	Facts           []materialityFact        `json:"facts"`
	EvidenceSupport capevent.EvidenceSupport `json:"evidence_support"`
}

type materialityFact struct {
	ID         string    `json:"id"`
	Type       EventType `json:"type"`
	OccurredAt string    `json:"occurred_at,omitempty"`
	Headline   string    `json:"headline"`
	Summary    string    `json:"summary"`
	Entities   []string  `json:"entities,omitempty"`
	Confidence float64   `json:"confidence"`
}

func (a *AgenticEventFactExtractor) Extract(
	ctx context.Context,
	task capevent.ResearchTask,
	documents []capevent.CanonicalDocument,
) (FactExtractionResult, error) {
	if a == nil || a.Model == nil {
		return FactExtractionResult{}, errors.New(
			"event fact extractor model is required",
		)
	}

	prompt := factExtractionPrompt{
		Symbol: task.Symbol,
		AsOf:   task.AsOf.UTC().Format(timeRFC3339),
		Documents: make(
			[]factExtractionDocument,
			0,
			len(documents),
		),
	}
	for _, document := range documents {
		item := factExtractionDocument{
			ID:           document.ID,
			Title:        document.Title,
			Content:      document.Content,
			Publisher:    document.Source.Publisher,
			SourceType:   document.Source.SourceType,
			Authority:    document.Source.Authority,
			EvidenceKind: document.Source.EvidenceKind,
			IsPrimary:    document.Source.IsPrimary,
		}
		if document.PublishedAt != nil {
			item.PublishedAt =
				document.PublishedAt.UTC().
					Format(timeRFC3339)
		}
		if document.UpdatedAt != nil {
			item.UpdatedAt =
				document.UpdatedAt.UTC().
					Format(timeRFC3339)
		}
		if document.AvailableAt != nil {
			item.AvailableAt =
				document.AvailableAt.UTC().
					Format(timeRFC3339)
		}
		prompt.Documents = append(
			prompt.Documents,
			item,
		)
	}

	var output FactExtractionResult
	if err := generateStrictAgenticJSON(
		ctx,
		a.Model,
		factExtractionInstruction,
		prompt,
		&output,
	); err != nil {
		return FactExtractionResult{}, fmt.Errorf(
			"event fact extraction model: %w",
			err,
		)
	}
	return output, nil
}

func (a *AgenticEventClusterer) Cluster(
	ctx context.Context,
	task capevent.ResearchTask,
	facts []Fact,
) (EventClusterResult, error) {
	if a == nil || a.Model == nil {
		return EventClusterResult{}, errors.New(
			"event clusterer model is required",
		)
	}

	var output EventClusterResult
	if err := generateStrictAgenticJSON(
		ctx,
		a.Model,
		eventClusteringInstruction,
		clusteringPrompt{
			Symbol: task.Symbol,
			Facts:  facts,
		},
		&output,
	); err != nil {
		return EventClusterResult{}, fmt.Errorf(
			"event clustering model: %w",
			err,
		)
	}
	return output, nil
}

func (a *AgenticEventMaterialityAnalyzer) AnalyzeMateriality(
	ctx context.Context,
	task capevent.ResearchTask,
	input MaterialityInput,
) (Materiality, error) {
	if a == nil || a.Model == nil {
		return Materiality{}, errors.New(
			"event materiality model is required",
		)
	}

	facts := make(
		[]materialityFact,
		0,
		len(input.Facts),
	)
	for _, fact := range input.Facts {
		facts = append(
			facts,
			materialityFact{
				ID:         fact.ID,
				Type:       fact.Type,
				OccurredAt: fact.OccurredAt,
				Headline:   fact.Headline,
				Summary:    fact.Summary,
				Entities:   fact.Entities,
				Confidence: fact.Confidence,
			},
		)
	}

	var output Materiality
	if err := generateStrictAgenticJSON(
		ctx,
		a.Model,
		materialityInstruction,
		materialityPrompt{
			Symbol:  task.Symbol,
			AsOf:    task.AsOf.UTC().Format(timeRFC3339),
			EventID:         input.EventID,
			Type:            input.Type,
			Facts:           facts,
			EvidenceSupport: input.EvidenceSupport,
		},
		&output,
	); err != nil {
		return Materiality{}, fmt.Errorf(
			"event materiality model: %w",
			err,
		)
	}
	return output, nil
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func generateStrictAgenticJSON(
	ctx context.Context,
	m model.BaseModel[*schema.AgenticMessage],
	instruction string,
	payload any,
	output any,
) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf(
			"encode model input: %w",
			err,
		)
	}

	response, err := m.Generate(
		ctx,
		[]*schema.AgenticMessage{
			schema.SystemAgenticMessage(
				instruction,
			),
			schema.UserAgenticMessage(
				string(data),
			),
		},
	)
	if err != nil {
		return fmt.Errorf(
			"generate model output: %w",
			err,
		)
	}

	raw := strings.TrimSpace(
		agenticMessageText(response),
	)
	if raw == "" {
		return errors.New(
			"model returned empty output",
		)
	}

	decoder := json.NewDecoder(
		strings.NewReader(raw),
	)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf(
			"decode strict JSON: %w: output=%q",
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
		return fmt.Errorf(
			"decode strict JSON: %w: output=%q",
			err,
			raw,
		)
	}

	return nil
}

func agenticMessageText(
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

const factExtractionInstruction = "You are the factual extraction stage of an investment research event analyst.\n\n" +
	"Extract only facts explicitly supported by the supplied canonical evidence.\n\n" +
	"Rules:\n" +
	"- Return JSON only.\n" +
	"- Do not assess whether an event is positive, negative, material, investable, or actionable.\n" +
	"- Do not add outside knowledge.\n" +
	"- Every fact must cite one or more supplied canonical document IDs.\n" +
	"- A document may yield zero, one, or multiple facts.\n" +
	"- Use occurred_at only when the evidence supports a concrete event time; otherwise leave it empty.\n" +
	"- occurred_at must be RFC3339 when present.\n" +
	"- confidence must be between 0 and 1.\n" +
	"- type must be one of DISCLOSURE, EARNINGS, GUIDANCE, CONTRACT, REGULATORY, LITIGATION, M_AND_A, FINANCING, DIVIDEND, BUYBACK, MANAGEMENT, PRODUCT, INCIDENT, POLICY, INDUSTRY, OTHER.\n" +
	"- fact summary must state what happened, not why the stock should rise or fall.\n\n" +
	"Output: {\"facts\":[{\"type\":\"CONTRACT\",\"occurred_at\":\"2026-09-29T10:00:00+08:00\",\"headline\":\"string\",\"summary\":\"string\",\"entities\":[\"string\"],\"evidence_document_ids\":[\"doc_x\"],\"confidence\":0.0}],\"confidence\":0.0}"

const eventClusteringInstruction = "You are the event clustering stage of an investment research system.\n\n" +
	"Group extracted facts that describe the same underlying real-world event.\n\n" +
	"Rules:\n" +
	"- Return JSON only.\n" +
	"- Use only the supplied fact IDs.\n" +
	"- Every supplied fact ID must appear exactly once across all clusters.\n" +
	"- Never add, delete, or rewrite facts.\n" +
	"- Do not assess materiality or market direction.\n" +
	"- Facts about different announcements, transactions, incidents, reporting periods, or regulatory actions must remain separate even if they concern the same company.\n" +
	"- Duplicate reports of the same underlying event should be grouped together.\n\n" +
	"Output: {\"clusters\":[{\"fact_ids\":[\"fact_x\",\"fact_y\"]}]}"

const materialityInstruction = "You are the interpretation stage of an investment research event analyst.\n\n" +
	"Assess the potential materiality of exactly one already-extracted factual event cluster.\n\n" +
	"Rules:\n" +
	"- Return JSON only.\n" +
	"- Do not rewrite or add facts.\n" +
	"- Use only the supplied facts.\n" +
	"- Do not output BUY, SELL, WATCH, AVOID, CANDIDATE, price targets, or trading instructions.\n" +
	"- level must be LOW, MEDIUM, HIGH, or UNKNOWN.\n" +
	"- direction must be POSITIVE, NEGATIVE, MIXED, NEUTRAL, or UNCERTAIN.\n" +
	"- horizon must be IMMEDIATE, SHORT_TERM, MEDIUM_TERM, or UNKNOWN.\n" +
	"- confidence must be between 0 and 1.\n" +
	"- EvidenceSupport describes provenance and corroboration strength, not economic materiality.\n" +
	"- Never raise or lower materiality level solely because EvidenceSupport changes.\n" +
	"- Use EvidenceSupport to calibrate epistemic confidence and identify evidence/provenance risks.\n" +
	"- PRIMARY_ONLY means primary evidence exists without cross-source corroboration; it is not inherently weak or immaterial.\n" +
	"- CORROBORATED means multiple independent source identities support the event; syndicated reprints have already been collapsed.\n" +
	"- If the supplied facts do not support a directional interpretation, use UNCERTAIN.\n" +
	"- rationale should explain the interpretation from the supplied facts.\n" +
	"- risks should capture uncertainty, missing context, or plausible interpretation limits.\n\n" +
	"Output: {\"level\":\"MEDIUM\",\"direction\":\"UNCERTAIN\",\"horizon\":\"SHORT_TERM\",\"rationale\":\"string\",\"risks\":[\"string\"],\"confidence\":0.0}"
