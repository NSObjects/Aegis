package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	eventagent "Aegis/internal/agent/event"
)

// RunEvidenceIndex is a compact projection of the EVENT context in the exact
// COMPLETED portfolio Artifact. It records event/document identities and PIT
// observation timestamps, not original documents, URLs, source authenticity,
// semantic corroboration or a list of tools that were actually invoked.
// A source_count value is a typed research result, not an independent audit.
type RunEvidenceIndex struct {
	SchemaVersion int `json:"schema_version"`
	Status string `json:"status"`
	RequestedAsOf string `json:"requested_as_of"`
	EvidenceAsOf string `json:"evidence_as_of,omitempty"`
	Events []RunEvidenceEvent `json:"events"`
	DocumentIDs []string `json:"document_ids"`
	// Present only when this run's Event Agent recorded canonical sources.
	Documents []eventagent.CitedDocumentSource `json:"documents,omitempty"`
}

type RunEvidenceEvent struct {
	ID string `json:"event_id"`
	OccurredAt string `json:"occurred_at,omitempty"`
	SourceCount int `json:"source_count"`
	DocumentIDs []string `json:"document_ids"`
}

// projectRunEvidenceIndex decodes only the typed EventContext in the saved
// result. Decision output has no EVENT context; nil means not captured.
// Old saved runs deliberately retain nil rather than receiving inferred data.
func projectRunEvidenceIndex(raw json.RawMessage, mode ResearchMode, symbol string, cutoff time.Time) (*RunEvidenceIndex, error) {
	if mode != ModePortfolio { return nil, nil }
	var payload struct {
		ResearchArtifact *struct {
			Symbol string `json:"symbol"`
			Event *eventagent.Context `json:"event"`
		} `json:"research_artifact"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode research evidence index: %w", err)
	}
	if payload.ResearchArtifact == nil || payload.ResearchArtifact.Event == nil { return nil, nil }
	if payload.ResearchArtifact.Symbol != symbol || payload.ResearchArtifact.Event.Symbol != symbol {
		return nil, errors.New("research evidence index symbol mismatch")
	}
	ctx := payload.ResearchArtifact.Event
	requested, err := time.Parse(time.RFC3339, ctx.RequestedAsOf)
	if err != nil || !requested.Equal(cutoff.Truncate(time.Second)) {
		return nil, errors.New("event evidence index cutoff mismatch")
	}
	if ctx.Status != eventagent.StatusNormal && ctx.Status != eventagent.StatusNoEvent &&
		ctx.Status != eventagent.StatusInsufficientEvidence {
		return nil, errors.New("invalid event evidence index status")
	}
	if (ctx.Status == eventagent.StatusNormal) != (len(ctx.Events) > 0) {
		return nil, errors.New("event evidence index status/events disagree")
	}
	if len(ctx.Events) > 256 { return nil, errors.New("too many indexed events") }
	if ctx.EvidenceAsOf != "" {
		if _, err := time.Parse(time.RFC3339, ctx.EvidenceAsOf); err != nil {
			return nil, errors.New("invalid evidence observation time")
		}
	}
	idx := &RunEvidenceIndex{
		SchemaVersion: 1, Status: string(ctx.Status),
		RequestedAsOf: ctx.RequestedAsOf, EvidenceAsOf: ctx.EvidenceAsOf,
		Events: []RunEvidenceEvent{}, DocumentIDs: []string{},
	}
	seenEvents := make(map[string]bool, len(ctx.Events))
	allDocs := make(map[string]struct{})
	for _, event := range ctx.Events {
		if !validIndexID(event.ID) || seenEvents[event.ID] {
			return nil, errors.New("ambiguous event index identity")
		}
		seenEvents[event.ID] = true
		if event.SourceCount < 0 { return nil, errors.New("negative recorded source count") }
		if event.OccurredAt != "" {
			if _, err := time.Parse(time.RFC3339, event.OccurredAt); err != nil {
				return nil, errors.New("invalid indexed event occurrence time")
			}
		}
		docs := make(map[string]struct{})
		for _, id := range event.EvidenceDocumentIDs {
			if !validIndexID(id) { return nil, errors.New("invalid indexed document id") }
			docs[id] = struct{}{}
		}
		// The Go validator already requires fact and event document coverage
		// to agree. Rechecking the union fails closed for invalid upstream
		// test fixtures or tampered raw outputs.
		for _, fact := range event.Facts {
			for _, id := range fact.EvidenceDocumentIDs {
				if !validIndexID(id) { return nil, errors.New("invalid fact document id") }
				docs[id] = struct{}{}
			}
			for _, grounding := range fact.Groundings {
				if !validIndexID(grounding.DocumentID) { return nil, errors.New("invalid grounding document id") }
				docs[grounding.DocumentID] = struct{}{}
			}
		}
		eventDocs := sortedDocumentIDs(docs)
		for _, id := range eventDocs { allDocs[id] = struct{}{} }
		idx.Events = append(idx.Events, RunEvidenceEvent{
			ID: event.ID, OccurredAt: event.OccurredAt,
			SourceCount: event.SourceCount, DocumentIDs: eventDocs,
		})
	}
	if len(allDocs) > 4096 { return nil, errors.New("too many indexed documents") }
	sort.Slice(idx.Events, func(i,j int) bool { return idx.Events[i].ID < idx.Events[j].ID })
	idx.DocumentIDs = sortedDocumentIDs(allDocs)
	if len(ctx.CitedDocumentSources) != 0 {
		if len(ctx.CitedDocumentSources) != len(idx.DocumentIDs) {
			return nil, errors.New("event index source metadata does not cover cited documents")
		}
		for i, source := range ctx.CitedDocumentSources {
			if source.DocumentID != idx.DocumentIDs[i] {
				return nil, errors.New("event index source citation identity mismatch")
			}
		}
		idx.Documents = append([]eventagent.CitedDocumentSource(nil), ctx.CitedDocumentSources...)
	}
	return idx, nil
}

func validIndexID(id string) bool {
	if id == "" || len(id) > 256 || strings.TrimSpace(id) != id { return false }
	return !strings.ContainsAny(id, "\r\n\x00")
}

func sortedDocumentIDs(input map[string]struct{}) []string {
	ids := make([]string, 0, len(input))
	for id := range input { ids = append(ids, id) }
	sort.Strings(ids)
	return ids
}

// A standalone metadata field must not diverge from the persisted Artifact.
// Legacy completed records without the index remain compatible; new records
// with an index must reproduce exactly from the same stored result bytes.
func checkRunEvidenceIndex(record RunRecord) error {
	if record.EvidenceIndex == nil { return nil }
	if record.State != RunCompleted || record.Mode != ModePortfolio ||
		record.EvidenceIndex.SchemaVersion != 1 {
		return errors.New("invalid evidence index lifecycle or schema")
	}
	expected, err := projectRunEvidenceIndex(record.Artifact, record.Mode, record.Symbol, record.RequestedAsOf)
	if err != nil { return err }
	if expected == nil || !reflect.DeepEqual(expected, record.EvidenceIndex) {
		return errors.New("research evidence index differs from saved artifact")
	}
	return nil
}
