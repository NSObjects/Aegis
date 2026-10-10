package event

import (
	capevent "Aegis/internal/capability/event"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
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

type AgenticEventContradictionDetector struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

var _ EventContradictionDetector =
	(*AgenticEventContradictionDetector)(nil)

type AgenticEventFactEvolutionDetector struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

var _ EventFactEvolutionDetector =
	(*AgenticEventFactEvolutionDetector)(nil)

type AgenticEventImpactMechanismAnalyzer struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

var _ EventImpactMechanismAnalyzer =
	(*AgenticEventImpactMechanismAnalyzer)(nil)

type AgenticEventMaterialityAnalyzer struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

var _ EventMaterialityAnalyzer = (*AgenticEventMaterialityAnalyzer)(nil)

type AgenticEventSynthesisAnalyzer struct { Model model.BaseModel[*schema.AgenticMessage] }
var _ EventSynthesisAnalyzer = (*AgenticEventSynthesisAnalyzer)(nil)

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

type contradictionPrompt struct {
	Symbol string `json:"symbol"`
	AsOf   string `json:"as_of"`
	Facts  []Fact `json:"facts"`
}

type factEvolutionPrompt struct {
	Symbol string `json:"symbol"`
	AsOf   string `json:"as_of"`
	Facts  []Fact `json:"facts"`
}

type impactMechanismPrompt struct {
	Symbol         string               `json:"symbol"`
	AsOf           string               `json:"as_of"`
	Facts          []Fact               `json:"facts"`
	Contradictions []ClaimContradiction `json:"contradictions,omitempty"`
	FactRelations  []FactRelation       `json:"fact_relations,omitempty"`
}

type synthesisPrompt struct { Symbol string `json:"symbol"`; AsOf string `json:"as_of"`; Events []Event `json:"events"` }

type materialityPrompt struct {
	Symbol          string                   `json:"symbol"`
	AsOf            string                   `json:"as_of"`
	EventID         string                   `json:"event_id"`
	Type            EventType                `json:"type"`
	Facts           []materialityFact        `json:"facts"`
	Contradictions  []ClaimContradiction     `json:"contradictions,omitempty"`
	FactRelations   []FactRelation           `json:"fact_relations,omitempty"`
	ImpactMechanisms []ImpactMechanism        `json:"impact_mechanisms,omitempty"`
	EvidenceSupport capevent.EvidenceSupport `json:"evidence_support"`
}

type materialityFact struct {
	ID         string              `json:"id"`
	Type       EventType           `json:"type"`
	OccurredAt string              `json:"occurred_at,omitempty"`
	Headline   string              `json:"headline"`
	Summary    string              `json:"summary"`
	Entities   []string            `json:"entities,omitempty"`
	Groundings []EvidenceGrounding `json:"groundings"`
	Confidence float64             `json:"confidence"`
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
	if groundingErr :=
		validateFactExtractionGroundings(
			output,
			documents,
		); groundingErr == nil {
		return output, nil
	} else {
		var repaired FactExtractionResult
		repairInstruction :=
			factExtractionInstruction +
				"\n\nThe previous extraction failed deterministic grounding validation: " +
				groundingErr.Error() +
				". Return the full corrected JSON. Use previous_extraction only as a candidate, not as evidence. Recheck every quote against documents[].content; omit unsupported facts, do not invent or paraphrase quotations. Every retained quote must occur exactly once verbatim in its identified canonical document. The grounding_quote_candidates are exact snippets copied from the indicated documents around repeated quotes, not additional evidence; select a candidate only if its whole context supports the fact, or omit the unsupported fact. Never invent offsets."
		if err := generateStrictAgenticJSON(
			ctx,
			a.Model,
			repairInstruction,
			struct { factExtractionPrompt; PreviousExtraction FactExtractionResult `json:"previous_extraction"`; GroundingQuoteCandidates []groundingRepairHint `json:"grounding_quote_candidates,omitempty"` }{factExtractionPrompt:prompt,PreviousExtraction:output,GroundingQuoteCandidates:groundingRepairHints(output,documents)},
			&repaired,
		); err != nil {
			return FactExtractionResult{}, fmt.Errorf(
				"event fact extraction grounding repair model: %w",
				err,
			)
		}
		if err := validateFactExtractionGroundings(
			repaired,
			documents,
		); err != nil {
			return FactExtractionResult{}, fmt.Errorf(
				"event fact extraction grounding repair: %w",
				err,
			)
		}
		return repaired, nil
	}
}

// groundingRepairHint is a verbatim, unambiguous source excerpt around a
// repeated model quote. It is guidance for another LLM attempt, never a
// validated grounding or a replacement for ResolveEvidenceGroundings.
type groundingRepairHint struct {
 DocumentID string `json:"document_id"`
 AmbiguousQuote string `json:"ambiguous_quote"`
 VerbatimCandidate string `json:"verbatim_candidate"`
}

func groundingRepairHints(result FactExtractionResult, documents []capevent.CanonicalDocument) []groundingRepairHint {
 byID:=make(map[string]string,len(documents))
 for _,doc:=range documents {byID[doc.ID]=doc.Content}
 var hints []groundingRepairHint
 seen:=make(map[string]struct{})
 for _,fact:=range result.Facts {
  for _,q:=range fact.EvidenceQuotes {
   content,exists:=byID[strings.TrimSpace(q.DocumentID)]
   quote:=strings.TrimSpace(q.Quote)
   if !exists||quote==""||strings.Index(content,quote)<0 {continue}
   positions:=make([]int,0,4)
   for offset:=0;offset<len(content); {
    ix:=strings.Index(content[offset:],quote)
    if ix<0 {break}
    start:=offset+ix
    positions=append(positions,start)
    if len(positions)>4 {break}
    offset=start+1
   }
   if len(positions)<2 || len(positions)>4 {continue}
   runes:=[]rune(content)
   quoteRunes:=len([]rune(quote))
   for _,pos:=range positions {
    runeStart:=len([]rune(content[:pos]))
    var candidate string
    for _,radius:=range []int{24,48,96,192,384} {
     lo:=runeStart-radius
     if lo<0 {lo=0}
     hi:=runeStart+quoteRunes+radius
     if hi>len(runes){hi=len(runes)}
     excerpt:=string(runes[lo:hi])
     first:=strings.Index(content,excerpt)
     if first>=0 && strings.Index(content[first+1:],excerpt)<0 {
      candidate=excerpt
      break
     }
    }
    if candidate=="" {continue}
    key:=q.DocumentID+"\x00"+quote+"\x00"+candidate
    if _,ok:=seen[key];ok {continue}
    seen[key]=struct{}{}
    hints=append(hints,groundingRepairHint{
     DocumentID:q.DocumentID,
     AmbiguousQuote:quote,
     VerbatimCandidate:candidate,
    })
   }
  }
 }
 return hints
}

func validateFactExtractionGroundings(
	extraction FactExtractionResult,
	documents []capevent.CanonicalDocument,
) error {
	documentByID := make(
		map[string]capevent.CanonicalDocument,
		len(documents),
	)
	for _, document := range documents {
		documentByID[document.ID] = document
	}
	for index, fact := range extraction.Facts {
		if _, err := ResolveEvidenceGroundings(
			fact.EvidenceQuotes,
			documentByID,
		); err != nil {
			return fmt.Errorf(
				"fact %d: %w",
				index,
				err,
			)
		}
	}
	return nil
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

func (a *AgenticEventContradictionDetector) DetectContradictions(
	ctx context.Context,
	task capevent.ResearchTask,
	facts []Fact,
) (ContradictionDetectionResult, error) {
	if a == nil || a.Model == nil {
		return ContradictionDetectionResult{}, errors.New(
			"event contradiction detector model is required",
		)
	}
	if len(facts) < 2 {
		return ContradictionDetectionResult{
			Contradictions: make(
				[]DetectedContradiction,
				0,
			),
		}, nil
	}

	var output ContradictionDetectionResult
	if err := generateStrictAgenticJSON(
		ctx,
		a.Model,
		contradictionDetectionInstruction,
		contradictionPrompt{
			Symbol: task.Symbol,
			AsOf: task.AsOf.UTC().
				Format(timeRFC3339),
			Facts: facts,
		},
		&output,
	); err != nil {
		return ContradictionDetectionResult{}, fmt.Errorf(
			"event contradiction detection model: %w",
			err,
		)
	}
	return output, nil
}

func (a *AgenticEventFactEvolutionDetector) DetectFactEvolution(
	ctx context.Context,
	task capevent.ResearchTask,
	facts []Fact,
) (FactEvolutionDetectionResult, error) {
	if a == nil || a.Model == nil {
		return FactEvolutionDetectionResult{}, errors.New(
			"event fact evolution detector model is required",
		)
	}
	if len(facts) < 2 {
		return FactEvolutionDetectionResult{Relations: []DetectedFactRelation{}}, nil
	}
	var output FactEvolutionDetectionResult
	if err := generateStrictAgenticJSON(
		ctx, a.Model, factEvolutionInstruction,
		factEvolutionPrompt{Symbol: task.Symbol, AsOf: task.AsOf.UTC().Format(timeRFC3339), Facts: facts},
		&output,
	); err != nil {
		return FactEvolutionDetectionResult{}, fmt.Errorf(
			"event fact evolution detection model: %w", err,
		)
	}
	return output, nil
}

func (a *AgenticEventImpactMechanismAnalyzer) AnalyzeImpactMechanisms(
	ctx context.Context,
	task capevent.ResearchTask,
	facts []Fact,
	contradictions []ClaimContradiction,
	factRelations []FactRelation,
) (ImpactMechanismDetectionResult, error) {
	if a == nil || a.Model == nil {
		return ImpactMechanismDetectionResult{}, errors.New("event impact mechanism analyzer model is required")
	}
	payload := impactMechanismPrompt{
		Symbol: task.Symbol, AsOf: task.AsOf.UTC().Format(timeRFC3339), Facts: facts,
		Contradictions: contradictions, FactRelations: factRelations,
	}
	instruction := impactMechanismInstruction
	for attempt := 0; attempt < 2; attempt++ {
		var output ImpactMechanismDetectionResult
		if err := generateStrictAgenticJSON(ctx, a.Model, instruction, payload, &output); err != nil {
			return ImpactMechanismDetectionResult{}, fmt.Errorf("event impact mechanism model: %w", err)
		}
		if err := validateDetectedImpactEnums(output.Mechanisms); err == nil {
			return output, nil
		} else if attempt == 1 {
			return ImpactMechanismDetectionResult{}, fmt.Errorf("event impact mechanism invalid enum after correction: %w", err)
		} else {
			instruction = impactMechanismInstruction + "\\n\\nYour previous output failed deterministic enum validation: " + err.Error() + ". Regenerate the complete JSON using only the exact channel and direction values specified above. Do not silently translate a market-price effect into a business mechanism; omit any mechanism without a supported permitted channel."
		}
	}
	return ImpactMechanismDetectionResult{}, errors.New("event impact mechanism correction attempts exhausted")
}

func validateDetectedImpactEnums(mechanisms []DetectedImpactMechanism) error {
	for i, mechanism := range mechanisms {
		if !validImpactChannel(mechanism.Channel) {
			return fmt.Errorf("impact mechanism %d has invalid channel %q", i, mechanism.Channel)
		}
		if !validImpactDirection(mechanism.Direction) {
			return fmt.Errorf("impact mechanism %d has invalid direction %q", i, mechanism.Direction)
		}
	}
	return nil
}

func (a *AgenticEventSynthesisAnalyzer) Synthesize(ctx context.Context, task capevent.ResearchTask, events []Event) (AnalystSynthesis,error) {
	if a==nil || a.Model==nil { return AnalystSynthesis{},errors.New("event synthesis analyzer model is required") }
	var output AnalystSynthesis
	if err:=generateStrictAgenticJSON(ctx,a.Model,synthesisInstruction,synthesisPrompt{Symbol:task.Symbol,AsOf:task.AsOf.UTC().Format(timeRFC3339),Events:events},&output);err!=nil { return AnalystSynthesis{},fmt.Errorf("event synthesis model: %w",err) }
	return output,nil
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
				Groundings: fact.Groundings,
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
			Contradictions:  input.Contradictions,
			FactRelations:   input.FactRelations,
			ImpactMechanisms:input.ImpactMechanisms,
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

const maxStrictJSONAttempts = 2

func generateStrictAgenticJSON(
	ctx context.Context,
	m model.BaseModel[*schema.AgenticMessage],
	instruction string,
	payload any,
	output any,
) error {
	target := reflect.ValueOf(output)
	if !target.IsValid() || target.Kind() != reflect.Pointer || target.IsNil() {
		return errors.New("strict JSON output must be a non-nil pointer")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode model input: %w", err)
	}
	for attempt := 1; attempt <= maxStrictJSONAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		prompt := instruction
		if attempt > 1 {
			prompt += "\nReturn a new valid JSON object with only the schema fields specified above. No wrappers or additional keys."
		}
		response, err := m.Generate(ctx, []*schema.AgenticMessage{
			schema.SystemAgenticMessage(prompt),
			schema.UserAgenticMessage(string(data)),
		})
		if err != nil {
			return fmt.Errorf("generate model output: %w", err)
		}
		raw := strings.TrimSpace(agenticMessageText(response))
		if raw == "" {
			return errors.New("model returned empty output")
		}
		candidate := reflect.New(target.Elem().Type())
		if err := decodeStrictJSON(raw, candidate.Interface()); err != nil {
			if attempt == maxStrictJSONAttempts {
				return fmt.Errorf("decode strict JSON after %d attempts: %w: output=%q",
					attempt, err, raw)
			}
			continue
		}
		target.Elem().Set(candidate.Elem())
		return nil
	}
	return errors.New("strict JSON attempts exhausted")
}

func decodeStrictJSON(raw string, output any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
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
	"- Every fact must include one or more evidence_quotes.\n" +
	"- Each evidence quote must contain a supplied canonical document_id and a verbatim quote copied exactly from that document's content.\n" +
	"- Never invent byte offsets; the runtime verifies the quote and derives offsets deterministically.\n" +
	"- Use a concise quote that directly supports the factual claim; do not paraphrase inside quote.\n" +
	"- Every quote must occur exactly once in its identified document content. If a short phrase is repeated, extend the quote with adjacent verbatim text until the occurrence is unique.\n" +
	"- Do not use ellipses, omitted text markers, normalized wording, or reconstructed sentences inside quote.\n" +
	"- A document may yield zero, one, or multiple facts.\n" +
	"- Use occurred_at only when the evidence supports a concrete event time; otherwise leave it empty.\n" +
	"- occurred_at must be RFC3339 when present.\n" +
	"- confidence must be between 0 and 1.\n" +
	"- type must be one of DISCLOSURE, EARNINGS, GUIDANCE, CONTRACT, REGULATORY, LITIGATION, M_AND_A, FINANCING, DIVIDEND, BUYBACK, MANAGEMENT, PRODUCT, INCIDENT, POLICY, INDUSTRY, OTHER.\n" +
	"- fact summary must state what happened, not why the stock should rise or fall.\n\n" +
	"Output: {\"facts\":[{\"type\":\"CONTRACT\",\"occurred_at\":\"2026-09-29T10:00:00+08:00\",\"headline\":\"string\",\"summary\":\"string\",\"entities\":[\"string\"],\"evidence_quotes\":[{\"document_id\":\"doc_x\",\"quote\":\"verbatim supporting text\"}],\"confidence\":0.0}],\"confidence\":0.0}"

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

const contradictionDetectionInstruction = "You are the claim contradiction detection stage of an investment research event analyst.\n\n" +
	"The supplied facts have already been grounded to exact evidence spans and clustered as the same underlying real-world event. Identify only factual claims that cannot both be true under the same relevant subject, event, time, and measurement scope.\n\n" +
	"Rules:\n" +
	"- Return JSON only.\n" +
	"- Use only supplied fact IDs.\n" +
	"- Do not decide which fact is true, more credible, newer, or better sourced.\n" +
	"- Do not output source quality, confidence scores, winners, or recommendations.\n" +
	"- Do not mark later update vs earlier state, different reporting periods, different measurement/accounting scopes, estimate vs actual, omission, partial information, or merely different compatible details as contradictions.\n" +
	"- A contradiction requires that both factual claims cannot simultaneously hold under the same scope.\n" +
	"- kind must be VALUE, STATUS, DATE, QUANTITY, IDENTITY, or OTHER.\n" +
	"- rationale must briefly state why the two claims are mutually incompatible without choosing a winner.\n" +
	"- Return each conflicting pair at most once.\n" +
	"- If there is no explicit contradiction, return an empty contradictions array.\n\n" +
	"Output: {\"contradictions\":[{\"left_fact_id\":\"fact_a\",\"right_fact_id\":\"fact_b\",\"kind\":\"VALUE\",\"rationale\":\"string\"}]}"

const factEvolutionInstruction = "You are the fact temporal evolution stage of an investment research event analyst.\n\n" +
	"The supplied grounded facts belong to the same event cluster. Identify only defensible directional evolution relationships between claims.\n\n" +
	"Rules:\n" +
	"- Return JSON only.\n" +
	"- Use only supplied fact IDs.\n" +
	"- from_fact_id is the later/evolving claim; to_fact_id is the earlier claim.\n" +
	"- UPDATES means a later claim updates an earlier claim for the same subject and scope.\n" +
	"- SUPERSEDES means a later claim explicitly replaces an earlier effective value or state.\n" +
	"- FINALIZES means a final or official claim resolves a preliminary or provisional claim.\n" +
	"- REALIZES means an actual result realizes a prior forecast or guidance claim.\n" +
	"- Do not create a relation merely because facts mention the same entity, occur at different times, are similar, have different values, or come from different source authorities.\n" +
	"- Different periods without an explicit evolution relationship are not related.\n" +
	"- Do not decide which fact is true or more credible. Recency is not truth.\n" +
	"- Do not convert an unresolved contradiction into an evolution relation merely to resolve it.\n" +
	"- rationale must explain the concrete evolution semantics without outside facts.\n" +
	"- If no defensible relation exists, return an empty relations array.\n\n" +
	"Output: {\"relations\":[{\"type\":\"UPDATES\",\"from_fact_id\":\"fact_new\",\"to_fact_id\":\"fact_old\",\"rationale\":\"string\"}]}"

const impactMechanismInstruction = "You are the causal impact-mechanism stage of an investment research event analyst.\n\n" +
	"Explain only defensible business/economic transmission channels from the supplied grounded event facts.\n\n" +
	"Rules:\n" +
	"- Return JSON only. Use only supplied fact IDs.\n" +
	"- channel must be REVENUE, PROFIT, CASH_FLOW, COST, CAPACITY, DEMAND, SUPPLY, FINANCING, OWNERSHIP, REGULATORY, OPERATIONS, or REPUTATION.\n" +
	"- direction must be POSITIVE, NEGATIVE, MIXED, NEUTRAL, or UNCERTAIN.\n" +
	"- Every mechanism must cite one or more supporting_fact_ids from the supplied event.\n" +
	"- A mechanism describes a business/economic transmission path, not a stock-price prediction.\n" +
	"- Do not invent margins, conversion rates, probabilities, timing, volumes, revenue recognition, costs, or other numeric assumptions not present in the facts.\n" +
	"- Do not assume a contract is profitable, a policy raises demand, financing is accretive, or reputation changes revenue unless the supplied facts support that step.\n" +
	"- Use UNCERTAIN when a plausible channel exists but its directional effect is not supported.\n" +
	"- Put material missing conditions or causal limits in uncertainty.\n" +
	"- Contradictions remain unresolved; do not choose a winning fact.\n" +
	"- FactRelations provide temporal semantics but do not prove the newer fact is true.\n" +
	"- Do not output BUY, SELL, WATCH, AVOID, CANDIDATE, price targets, or trading instructions.\n" +
	"- If the supplied facts do not support a defensible mechanism, return an empty mechanisms array.\n\n" +
	"Output: {\"mechanisms\":[{\"channel\":\"REVENUE\",\"direction\":\"POSITIVE\",\"supporting_fact_ids\":[\"fact_a\"],\"rationale\":\"string\",\"uncertainty\":\"string\"}]}"

const synthesisInstruction = "You are the final synthesis stage of a News/Event investment research analyst.\n\n" +
	"Summarize only the supplied already-validated event objects. Do not add facts, evidence, causal steps, or outside knowledge.\n\n" +
	"Rules:\n" +
	"- Return JSON only.\n" +
	"- key_event_ids must contain only supplied event IDs and identify the events that actually drive the conclusion.\n" +
	"- overall_materiality must be LOW, MEDIUM, HIGH, or UNKNOWN.\n" +
	"- overall_direction must be POSITIVE, NEGATIVE, MIXED, NEUTRAL, or UNCERTAIN.\n" +
	"- dominant_impact_channels may contain only channels already present in supplied events' impact_mechanisms.\n" +
	"- key_risks must be grounded in supplied materiality risks, contradictions, impact uncertainty, or evidence limitations.\n" +
	"- Never resolve an unresolved contradiction by choosing a winning fact.\n" +
	"- Never extend an impact mechanism with an unsupported causal step.\n" +
	"- Do not output BUY, SELL, WATCH, AVOID, CANDIDATE, price targets, expected returns, or portfolio actions.\n" +
	"- confidence is epistemic confidence in this synthesis, between 0 and 1; it is not a probability of price movement.\n\n" +
	"Output: {\"summary\":\"string\",\"overall_materiality\":\"MEDIUM\",\"overall_direction\":\"UNCERTAIN\",\"dominant_impact_channels\":[\"REVENUE\"],\"key_event_ids\":[\"event_id\"],\"key_risks\":[\"string\"],\"confidence\":0.7}"

const materialityInstruction = "You are the interpretation stage of an investment research event analyst.\n\n" +
	"Assess the potential materiality of exactly one already-extracted factual event cluster.\n\n" +
	"Rules:\n" +
	"- Return JSON only.\n" +
	"- Do not rewrite or add facts.\n" +
	"- Use only the supplied facts.\n" +
	"- Fact groundings are already runtime-verified exact spans. Treat them as provenance for the supplied fact, not as permission to infer additional facts from surrounding or outside text.\n" +
	"- Contradictions are unresolved mutually incompatible grounded claims. Do not choose a winning claim.\n" +
	"- Contradiction presence is epistemic uncertainty, not economic materiality; never raise or lower materiality level solely because a contradiction exists.\n" +
	"- FactRelations describe temporal/evolution semantics only. Relation existence is not economic materiality and does not prove the later claim is true.\n" +
	"- ImpactMechanisms are runtime-grounded transmission hypotheses. Use them to explain supported business/economic pathways, but mechanism existence alone is not materiality.\n" +
	"- Do not extend ImpactMechanisms with unsupported causal steps or numeric assumptions.\n" +
	"- Use FactRelations only to distinguish earlier forecast/preliminary/state claims from later actual/final/update claims when the supplied relation supports that interpretation.\n" +
	"- If an unresolved contradiction prevents a supported directional interpretation, use UNCERTAIN and expose the conflict in risks.\n" +
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
