package event

import (
	capevent "Aegis/internal/capability/event"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type ExtractedFact struct {
	Type           EventType       `json:"type"`
	OccurredAt     string          `json:"occurred_at,omitempty"`
	Headline       string          `json:"headline"`
	Summary        string          `json:"summary"`
	Entities       []string        `json:"entities,omitempty"`
	EvidenceQuotes []EvidenceQuote `json:"evidence_quotes"`
	Confidence     float64         `json:"confidence"`
}

type FactExtractionResult struct {
	Facts      []ExtractedFact `json:"facts"`
	Confidence float64         `json:"confidence"`
}

type Fact struct {
	ID                  string              `json:"id"`
	Type                EventType           `json:"type"`
	OccurredAt          string              `json:"occurred_at,omitempty"`
	Headline            string              `json:"headline"`
	Summary             string              `json:"summary"`
	Entities            []string            `json:"entities,omitempty"`
	Groundings          []EvidenceGrounding `json:"groundings"`
	EvidenceDocumentIDs []string            `json:"evidence_document_ids"`
	Confidence          float64             `json:"confidence"`
}

type EventCluster struct {
	FactIDs []string `json:"fact_ids"`
}

type EventClusterResult struct {
	Clusters []EventCluster `json:"clusters"`
}

type MaterialityInput struct {
	EventID         string
	Type            EventType
	Facts           []Fact
	Contradictions  []ClaimContradiction
	FactRelations   []FactRelation
	ImpactMechanisms []ImpactMechanism
	EvidenceSupport capevent.EvidenceSupport
}

type EventFactExtractor interface {
	Extract(
		ctx context.Context,
		task capevent.ResearchTask,
		documents []capevent.CanonicalDocument,
	) (FactExtractionResult, error)
}

type EventClusterer interface {
	Cluster(
		ctx context.Context,
		task capevent.ResearchTask,
		facts []Fact,
	) (EventClusterResult, error)
}

type EventContradictionDetector interface {
	DetectContradictions(
		ctx context.Context,
		task capevent.ResearchTask,
		facts []Fact,
	) (ContradictionDetectionResult, error)
}

type EventFactEvolutionDetector interface {
	DetectFactEvolution(
		ctx context.Context,
		task capevent.ResearchTask,
		facts []Fact,
	) (FactEvolutionDetectionResult, error)
}

type EventImpactMechanismAnalyzer interface {
	AnalyzeImpactMechanisms(
		ctx context.Context,
		task capevent.ResearchTask,
		facts []Fact,
		contradictions []ClaimContradiction,
		factRelations []FactRelation,
	) (ImpactMechanismDetectionResult, error)
}

type EventMaterialityAnalyzer interface {
	AnalyzeMateriality(
		ctx context.Context,
		task capevent.ResearchTask,
		input MaterialityInput,
	) (Materiality, error)
}

type AnalyzeInput struct {
	Task           capevent.ResearchTask
	ResearchStatus capevent.ResearchTaskStatus
	Documents      []capevent.CanonicalDocument
	Corroboration  capevent.CorroborationResult
}

type Analyzer struct {
	Extractor      EventFactExtractor
	Clusterer      EventClusterer
	Contradiction EventContradictionDetector
	Evolution     EventFactEvolutionDetector
	Impact        EventImpactMechanismAnalyzer
	Materiality    EventMaterialityAnalyzer
}

func (a Analyzer) Analyze(
	ctx context.Context,
	input AnalyzeInput,
) (*Context, error) {
	task := input.Task
	task.Symbol = strings.TrimSpace(task.Symbol)
	if task.Symbol == "" {
		return nil, errors.New(
			"event analyzer symbol is required",
		)
	}
	if task.AsOf.IsZero() {
		return nil, errors.New(
			"event analyzer as_of is required",
		)
	}

	switch input.ResearchStatus {
	case capevent.TaskSatisfied:
		// Continue into event understanding.
	case capevent.TaskPartial,
		capevent.TaskUnsatisfied:
		result := &Context{
			Symbol:        task.Symbol,
			RequestedAsOf: task.AsOf.UTC().Format(time.RFC3339),
			Status:        StatusInsufficientEvidence,
			Risk: []string{
				fmt.Sprintf(
					"upstream event evidence status is %s",
					input.ResearchStatus,
				),
			},
			Confidence: 0,
		}
		if err := Validate(result); err != nil {
			return nil, err
		}
		return result, nil
	default:
		return nil, fmt.Errorf(
			"event analyzer has invalid upstream research status %q",
			input.ResearchStatus,
		)
	}

	documentByID, err := validateAnalyzerDocuments(
		task,
		input.Documents,
	)
	if err != nil {
		return nil, err
	}

	corroboration := input.Corroboration
	if len(corroboration.Clusters) == 0 &&
		len(input.Documents) > 0 {
		corroboration =
			(capevent.EvidenceCorroborator{}).
				Corroborate(input.Documents)
	}

	if a.Extractor == nil {
		return nil, errors.New(
			"event fact extractor is required",
		)
	}

	extraction, err := a.Extractor.Extract(
		ctx,
		task,
		input.Documents,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"extract event facts: %w",
			err,
		)
	}
	if extraction.Confidence < 0 ||
		extraction.Confidence > 1 {
		return nil, fmt.Errorf(
			"event extraction confidence must be between 0 and 1: %f",
			extraction.Confidence,
		)
	}

	facts, err := normalizeExtractedFacts(
		extraction.Facts,
		documentByID,
	)
	if err != nil {
		return nil, err
	}

	if len(facts) == 0 {
		result := &Context{
			Symbol:        task.Symbol,
			RequestedAsOf: task.AsOf.UTC().Format(time.RFC3339),
			Status:        StatusNoEvent,
			Confidence:    extraction.Confidence,
		}
		if err := Validate(result); err != nil {
			return nil, err
		}
		return result, nil
	}

	if a.Clusterer == nil {
		return nil, errors.New(
			"event clusterer is required",
		)
	}
	clusterResult, err := a.Clusterer.Cluster(
		ctx,
		task,
		facts,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"cluster event facts: %w",
			err,
		)
	}

	clusters, err := validateClusters(
		facts,
		clusterResult.Clusters,
	)
	if err != nil {
		return nil, err
	}
	if a.Materiality == nil {
		return nil, errors.New(
			"event materiality analyzer is required",
		)
	}

	events := make(
		[]Event,
		0,
		len(clusters),
	)
	for _, cluster := range clusters {
		clusterFacts := factsForCluster(
			facts,
			cluster.FactIDs,
		)
		eventID := deterministicEventID(
			cluster.FactIDs,
		)
		item := buildEventFactView(
			eventID,
			clusterFacts,
			documentByID,
		)

		if len(clusterFacts) > 1 {
			if a.Contradiction == nil {
				return nil, errors.New(
					"event contradiction detector is required for multi-fact event clusters",
				)
			}
			detection, detectErr :=
				a.Contradiction.DetectContradictions(
					ctx,
					task,
					clusterFacts,
				)
			if detectErr != nil {
				return nil, fmt.Errorf(
					"detect event %q contradictions: %w",
					eventID,
					detectErr,
				)
			}
			contradictions, normalizeErr :=
				normalizeContradictions(
					detection.Contradictions,
					clusterFacts,
					corroboration,
				)
			if normalizeErr != nil {
				return nil, fmt.Errorf(
					"normalize event %q contradictions: %w",
					eventID,
					normalizeErr,
				)
			}
			item.Contradictions = contradictions

			if a.Evolution == nil {
				return nil, errors.New(
					"event fact evolution detector is required for multi-fact event clusters",
				)
			}
			evolution, evolutionErr := a.Evolution.DetectFactEvolution(
				ctx, task, clusterFacts,
			)
			if evolutionErr != nil {
				return nil, fmt.Errorf(
					"detect event %q fact evolution: %w", eventID, evolutionErr,
				)
			}
			relations, relationErr := normalizeFactRelations(
				evolution.Relations, clusterFacts,
			)
			if relationErr != nil {
				return nil, fmt.Errorf(
					"normalize event %q fact evolution: %w", eventID, relationErr,
				)
			}
			item.FactRelations = relations
		}

		if a.Impact == nil {
			return nil, errors.New("event impact mechanism analyzer is required")
		}
		impactDetection, impactErr := a.Impact.AnalyzeImpactMechanisms(
			ctx, task, clusterFacts, item.Contradictions, item.FactRelations,
		)
		if impactErr != nil {
			return nil, fmt.Errorf("analyze event %q impact mechanisms: %w", eventID, impactErr)
		}
		mechanisms, mechanismErr := normalizeImpactMechanisms(impactDetection.Mechanisms, clusterFacts)
		if mechanismErr != nil {
			return nil, fmt.Errorf("normalize event %q impact mechanisms: %w", eventID, mechanismErr)
		}
		item.ImpactMechanisms = mechanisms

		support, supportErr :=
			(capevent.EvidenceSupportAnalyzer{}).
				Analyze(
					item.EvidenceDocumentIDs,
					input.Documents,
					corroboration,
				)
		if supportErr != nil {
			return nil, fmt.Errorf(
				"analyze event %q evidence support: %w",
				eventID,
				supportErr,
			)
		}
		item.EvidenceSupport = support
		item.SourceCount =
			support.IndependentSourceCount
		item.HasPrimarySource =
			support.PrimarySourcePresent

		materiality, analyzeErr :=
			a.Materiality.AnalyzeMateriality(
				ctx,
				task,
				MaterialityInput{
					EventID:         eventID,
					Type:            item.Type,
					Facts:           clusterFacts,
					Contradictions:  item.Contradictions,
					FactRelations:   item.FactRelations,
					ImpactMechanisms:item.ImpactMechanisms,
					EvidenceSupport: support,
				},
			)
		if analyzeErr != nil {
			return nil, fmt.Errorf(
				"analyze event %q materiality: %w",
				eventID,
				analyzeErr,
			)
		}
		if err := validateMateriality(
			materiality,
		); err != nil {
			return nil, fmt.Errorf(
				"event %q materiality: %w",
				eventID,
				err,
			)
		}
		item.Materiality = materiality
		events = append(events, item)
	}

	sort.SliceStable(
		events,
		func(i int, j int) bool {
			if events[i].OccurredAt !=
				events[j].OccurredAt {
				return events[i].OccurredAt >
					events[j].OccurredAt
			}
			return events[i].ID <
				events[j].ID
		},
	)

	result := &Context{
		Symbol:        task.Symbol,
		RequestedAsOf: task.AsOf.UTC().Format(time.RFC3339),
		Status:        StatusNormal,
		Events:        events,
		Evidence:      contextEvidenceIDs(events),
		Risk:          contextRisks(events),
		Confidence:    contextConfidence(events),
	}
	if err := Validate(result); err != nil {
		return nil, fmt.Errorf(
			"validate event context: %w",
			err,
		)
	}
	return result, nil
}

func validateAnalyzerDocuments(
	task capevent.ResearchTask,
	documents []capevent.CanonicalDocument,
) (map[string]capevent.CanonicalDocument, error) {
	byID := make(
		map[string]capevent.CanonicalDocument,
		len(documents),
	)
	for _, document := range documents {
		document.ID = strings.TrimSpace(document.ID)
		if document.ID == "" {
			return nil, errors.New(
				"event analyzer canonical document id is required",
			)
		}
		if _, duplicate := byID[document.ID]; duplicate {
			return nil, fmt.Errorf(
				"event analyzer canonical document id %q is duplicated",
				document.ID,
			)
		}
		if task.Requirements.PITSafe {
			if document.AvailableAt == nil {
				return nil, fmt.Errorf(
					"event analyzer PIT-safe document %q has no available_at",
					document.ID,
				)
			}
			if document.AvailableAt.After(task.AsOf) {
				return nil, fmt.Errorf(
					"event analyzer PIT-safe document %q is available after as_of",
					document.ID,
				)
			}
		}
		byID[document.ID] = document
	}
	return byID, nil
}

func normalizeExtractedFacts(
	extracted []ExtractedFact,
	documents map[string]capevent.CanonicalDocument,
) ([]Fact, error) {
	facts := make([]Fact, 0, len(extracted))
	seenIDs := make(map[string]struct{})

	for index, raw := range extracted {
		raw.Headline = strings.TrimSpace(
			raw.Headline,
		)
		raw.Summary = strings.TrimSpace(
			raw.Summary,
		)
		if !validEventType(raw.Type) {
			return nil, fmt.Errorf(
				"extracted fact %d has invalid type %q",
				index,
				raw.Type,
			)
		}
		if raw.Headline == "" {
			return nil, fmt.Errorf(
				"extracted fact %d headline is required",
				index,
			)
		}
		if raw.Summary == "" {
			return nil, fmt.Errorf(
				"extracted fact %d summary is required",
				index,
			)
		}
		if raw.Confidence < 0 ||
			raw.Confidence > 1 {
			return nil, fmt.Errorf(
				"extracted fact %d confidence must be between 0 and 1",
				index,
			)
		}
		if raw.OccurredAt != "" {
			if _, err := time.Parse(
				time.RFC3339,
				raw.OccurredAt,
			); err != nil {
				return nil, fmt.Errorf(
					"extracted fact %d occurred_at must be RFC3339: %w",
					index,
					err,
				)
			}
		}

		groundings, groundingErr :=
			ResolveEvidenceGroundings(
				raw.EvidenceQuotes,
				documents,
			)
		if groundingErr != nil {
			return nil, fmt.Errorf(
				"extracted fact %d grounding: %w",
				index,
				groundingErr,
			)
		}
		evidenceIDs := groundingDocumentIDs(
			groundings,
		)

		fact := Fact{
			Type:                raw.Type,
			OccurredAt:          strings.TrimSpace(raw.OccurredAt),
			Headline:            raw.Headline,
			Summary:             raw.Summary,
			Entities:            uniqueSortedStrings(raw.Entities),
			Groundings:          groundings,
			EvidenceDocumentIDs: evidenceIDs,
			Confidence:          raw.Confidence,
		}
		fact.ID = deterministicFactID(fact)
		if _, duplicate := seenIDs[fact.ID]; duplicate {
			continue
		}
		seenIDs[fact.ID] = struct{}{}
		facts = append(facts, fact)
	}

	sort.SliceStable(
		facts,
		func(i int, j int) bool {
			return facts[i].ID < facts[j].ID
		},
	)
	return facts, nil
}

func validateClusters(
	facts []Fact,
	clusters []EventCluster,
) ([]EventCluster, error) {
	expected := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		expected[fact.ID] = struct{}{}
	}

	seen := make(map[string]struct{}, len(facts))
	normalized := make(
		[]EventCluster,
		0,
		len(clusters),
	)
	for index, cluster := range clusters {
		factIDs := uniqueSortedStrings(
			cluster.FactIDs,
		)
		if len(factIDs) == 0 {
			return nil, fmt.Errorf(
				"event cluster %d must contain at least one fact",
				index,
			)
		}
		if len(factIDs) != len(cluster.FactIDs) {
			return nil, fmt.Errorf(
				"event cluster %d contains duplicate fact ids",
				index,
			)
		}
		for _, factID := range factIDs {
			if _, ok := expected[factID]; !ok {
				return nil, fmt.Errorf(
					"event cluster %d references unknown fact %q",
					index,
					factID,
				)
			}
			if _, duplicate := seen[factID]; duplicate {
				return nil, fmt.Errorf(
					"event fact %q appears in multiple clusters",
					factID,
				)
			}
			seen[factID] = struct{}{}
		}
		normalized = append(
			normalized,
			EventCluster{
				FactIDs: factIDs,
			},
		)
	}

	for factID := range expected {
		if _, ok := seen[factID]; !ok {
			return nil, fmt.Errorf(
				"event cluster coverage is missing fact %q",
				factID,
			)
		}
	}

	sort.SliceStable(
		normalized,
		func(i int, j int) bool {
			return strings.Join(
				normalized[i].FactIDs,
				"\x00",
			) < strings.Join(
				normalized[j].FactIDs,
				"\x00",
			)
		},
	)
	return normalized, nil
}

func deterministicFactID(
	fact Fact,
) string {
	parts := []string{
		string(fact.Type),
		fact.OccurredAt,
		strings.ToLower(
			strings.TrimSpace(fact.Summary),
		),
		strings.Join(
			fact.EvidenceDocumentIDs,
			"\x00",
		),
	}
	return shortHashID(
		"fact",
		strings.Join(parts, "\x00"),
	)
}

func deterministicEventID(
	factIDs []string,
) string {
	ids := append([]string(nil), factIDs...)
	sort.Strings(ids)
	return shortHashID(
		"event",
		strings.Join(ids, "\x00"),
	)
}

func shortHashID(
	prefix string,
	value string,
) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "_" +
		hex.EncodeToString(sum[:8])
}

func factsForCluster(
	facts []Fact,
	factIDs []string,
) []Fact {
	wanted := make(map[string]struct{}, len(factIDs))
	for _, factID := range factIDs {
		wanted[factID] = struct{}{}
	}

	selected := make([]Fact, 0, len(factIDs))
	for _, fact := range facts {
		if _, ok := wanted[fact.ID]; ok {
			selected = append(selected, fact)
		}
	}
	sort.SliceStable(
		selected,
		func(i int, j int) bool {
			left, leftOK := parsedFactTime(
				selected[i].OccurredAt,
			)
			right, rightOK := parsedFactTime(
				selected[j].OccurredAt,
			)
			if leftOK && rightOK &&
				!left.Equal(right) {
				return left.Before(right)
			}
			if leftOK != rightOK {
				return leftOK
			}
			return selected[i].ID <
				selected[j].ID
		},
	)
	return selected
}

func buildEventFactView(
	eventID string,
	facts []Fact,
	documents map[string]capevent.CanonicalDocument,
) Event {
	eventType := facts[0].Type
	sameType := true
	headline := facts[0].Headline
	summaries := make([]string, 0, len(facts))
	entities := make([]string, 0)
	evidenceIDs := make([]string, 0)
	confidenceSum := 0.0
	occurredAt := ""

	for _, fact := range facts {
		if fact.Type != eventType {
			sameType = false
		}
		summaries = append(
			summaries,
			fact.Summary,
		)
		entities = append(
			entities,
			fact.Entities...,
		)
		evidenceIDs = append(
			evidenceIDs,
			fact.EvidenceDocumentIDs...,
		)
		confidenceSum += fact.Confidence
		if occurredAt == "" &&
			fact.OccurredAt != "" {
			occurredAt = fact.OccurredAt
		}
	}
	if !sameType {
		eventType = EventOther
	}

	evidenceIDs = uniqueSortedStrings(
		evidenceIDs,
	)
	return Event{
		ID:                  eventID,
		Type:                eventType,
		OccurredAt:          occurredAt,
		Headline:            headline,
		FactSummary:         strings.Join(uniqueStringsPreserveOrder(summaries), "\n"),
		Entities:            uniqueSortedStrings(entities),
		Facts:               append([]Fact(nil), facts...),
		EvidenceDocumentIDs: evidenceIDs,
		SourceCount:         0,
		HasPrimarySource:    false,
		FactConfidence:      confidenceSum / float64(len(facts)),
	}
}

func contextEvidenceIDs(
	events []Event,
) []string {
	values := make([]string, 0)
	for _, item := range events {
		values = append(
			values,
			item.EvidenceDocumentIDs...,
		)
	}
	return uniqueSortedStrings(values)
}

func contextRisks(
	events []Event,
) []string {
	values := make([]string, 0)
	for _, item := range events {
		values = append(
			values,
			item.Materiality.Risks...,
		)
	}
	return uniqueStringsPreserveOrder(values)
}

func contextConfidence(
	events []Event,
) float64 {
	if len(events) == 0 {
		return 0
	}

	sum := 0.0
	for _, item := range events {
		sum += (item.FactConfidence +
			item.Materiality.Confidence) / 2
	}
	return sum / float64(len(events))
}

func uniqueSortedStrings(
	values []string,
) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func uniqueStringsPreserveOrder(
	values []string,
) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parsedFactTime(
	value string,
) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(
		time.RFC3339,
		value,
	)
	return parsed, err == nil
}
