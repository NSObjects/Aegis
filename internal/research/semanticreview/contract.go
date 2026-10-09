// Package semanticreview defines an advisory, offline contract for checking
// Insight-to-argument semantic consistency. It neither infers verdicts nor
// invokes models. This package must not control trading or portfolio policy.
package semanticreview

import (
	"Aegis/internal/research/debate"
	"Aegis/internal/research/evidenceledger"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/synthesis"
	"errors"
	"fmt"
	"strings"
)

// Task carries the exact source claims and typed references for one explicitly
// linked Manager Insight and Debate argument. Its alignment is a reference
// overlap diagnostic, NOT a verdict on semantic consistency.
type Task struct {
	InsightIndex        int                               `json:"insight_index"`
	InsightKind         evidenceledger.InsightKind        `json:"insight_kind"`
	InsightClaim        string                            `json:"insight_claim"`
	InsightReferences   []synthesis.Reference             `json:"insight_references"`
	ArgumentID          string                            `json:"argument_id"`
	ArgumentSide        debate.Side                       `json:"argument_side"`
	ArgumentClaim       string                            `json:"argument_claim"`
	ArgumentReferences  []synthesis.Reference             `json:"argument_references"`
	ReferenceAlignment  evidenceledger.ReferenceAlignment `json:"reference_alignment"`
}

type Tasks struct {
	Symbol string `json:"symbol"`
	Items  []Task `json:"items"`
}

// BuildTasks makes one deterministic task for every explicit Manager
// Insight→Debate argument link, validating all upstream contracts first.
// It never generates a semantic verdict and does not alter source data.
func BuildTasks(
	artifact *synthesis.Artifact,
	state *debate.State,
	plan *manager.ResearchPlan,
) (*Tasks, error) {
	ledger, err := evidenceledger.Build(artifact, state, plan)
	if err != nil {
		return nil, fmt.Errorf("build semantic review inputs: %w", err)
	}
	arguments := make(map[string]evidenceledger.Argument, len(ledger.Arguments))
	for _, arg := range ledger.Arguments {
		arguments[arg.ID] = arg
	}
	count := 0
	for _, insight := range ledger.Insights {
		count += len(insight.ArgumentReferenceAudits)
	}
	result := &Tasks{Symbol: ledger.Symbol, Items: make([]Task, 0, count)}
	for insightIndex, insight := range ledger.Insights {
		for _, audit := range insight.ArgumentReferenceAudits {
			argument, ok := arguments[audit.ArgumentID]
			if !ok {
				return nil, fmt.Errorf("semantic review argument %q missing from validated ledger", audit.ArgumentID)
			}
			result.Items = append(result.Items, Task{
				InsightIndex: insightIndex,
				InsightKind: insight.Kind,
				InsightClaim: insight.Claim,
				InsightReferences: append([]synthesis.Reference(nil), insight.References...),
				ArgumentID: argument.ID,
				ArgumentSide: argument.Side,
				ArgumentClaim: argument.Claim,
				ArgumentReferences: append([]synthesis.Reference(nil), argument.References...),
				ReferenceAlignment: audit.ReferenceAlignment,
			})
		}
	}
	return result, nil
}

// Verdict is a caller-supplied advisory opinion, never computed from
// reference alignment. Even CONSISTENT does not certify truth or evidence.
type Verdict string

const (
	VerdictConsistent    Verdict = "CONSISTENT"
	VerdictContradictory Verdict = "CONTRADICTORY"
	VerdictUncertain     Verdict = "UNCERTAIN"
)

type Finding struct {
	InsightIndex int     `json:"insight_index"`
	ArgumentID   string  `json:"argument_id"`
	Verdict      Verdict `json:"verdict"`
	Rationale    string  `json:"rationale"`
	Limitations  string  `json:"limitations,omitempty"`
}

// Report is authored by a future independent reviewer or human. A successful
// validation means only that the shape and IDs match, never that the opinion
// is semantically or factually correct.
type Report struct {
	Symbol   string    `json:"symbol"`
	Findings []Finding `json:"findings"`
}

// ValidateReport requires exact one-to-one ordered coverage. It deliberately
// does not invent missing assessments, infer verdicts, or gate another stage.
func ValidateReport(tasks *Tasks, report *Report) error {
	if tasks == nil {
		return errors.New("semantic review tasks are nil")
	}
	if report == nil {
		return errors.New("semantic review report is nil")
	}
	if strings.TrimSpace(tasks.Symbol) == "" || strings.TrimSpace(report.Symbol) != tasks.Symbol {
		return fmt.Errorf("semantic review symbol %q does not match tasks %q", report.Symbol, tasks.Symbol)
	}
	if len(report.Findings) != len(tasks.Items) {
		return fmt.Errorf("semantic review finding count %d does not match task count %d", len(report.Findings), len(tasks.Items))
	}
	for i, task := range tasks.Items {
		finding := report.Findings[i]
		if finding.InsightIndex != task.InsightIndex {
			return fmt.Errorf("finding %d insight_index %d does not match task %d", i, finding.InsightIndex, task.InsightIndex)
		}
		if finding.ArgumentID != task.ArgumentID {
			return fmt.Errorf("finding %d argument_id %q does not match task %q", i, finding.ArgumentID, task.ArgumentID)
		}
		switch finding.Verdict {
		case VerdictConsistent, VerdictContradictory, VerdictUncertain:
		default:
			return fmt.Errorf("finding %d has unsupported verdict %q", i, finding.Verdict)
		}
		if strings.TrimSpace(finding.Rationale) == "" {
			return fmt.Errorf("finding %d rationale is required", i)
		}
		if finding.Verdict == VerdictUncertain && strings.TrimSpace(finding.Limitations) == "" {
			return fmt.Errorf("finding %d limitations required for UNCERTAIN", i)
		}
	}
	return nil
}
