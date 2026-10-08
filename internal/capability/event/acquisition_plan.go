package event

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type AcquisitionScope string

const (
	AcquisitionScopeDirect    AcquisitionScope = "direct"
	AcquisitionScopeCandidate AcquisitionScope = "candidate"
)

// EvidenceSelectionDecision is an auditable relevance decision for one raw
// candidate document. Candidate evidence cannot enter the root evidence pool
// without one explicit decision.
type EvidenceSelectionMethod string

const (
	SelectionDeterministic EvidenceSelectionMethod = "deterministic"
	SelectionSemantic      EvidenceSelectionMethod = "semantic"
	// SelectionUnreviewed means the semantic matcher did not return a
	// decision for this candidate. The selection is a fail-closed rejection,
	// not a model judgment that the document is irrelevant.
	SelectionUnreviewed EvidenceSelectionMethod = "unreviewed"
)

type EvidenceSelectionDecision struct {
	RawDocumentID string
	Selected      bool
	Method        EvidenceSelectionMethod
	Confidence    float64
	Reason        string
}

// EvidenceSelector decides whether broad candidate evidence is relevant to the
// root ResearchTask. V2.7.4 defines only the boundary; the production semantic
// selector is intentionally deferred.
type EvidenceSelector interface {
	Select(
		ctx context.Context,
		task ResearchTask,
		documents []RawDocument,
	) ([]EvidenceSelectionDecision, error)
}

// AcquisitionStep wires one concrete Retriever to a retriever-specific task.
// TaskRunner remains the leaf executor; this type only describes orchestration.
type AcquisitionStep struct {
	ID        string
	Scope     AcquisitionScope
	Task      ResearchTask
	Retriever Retriever
}

type AcquisitionPlan struct {
	Steps    []AcquisitionStep
	Selector EvidenceSelector
}

type AcquisitionStepOutput struct {
	StepID       string
	Scope        AcquisitionScope
	Task         ResearchTask
	Run          TaskRunOutput
	ErrorMessage string
}

type AcquisitionPlanOutput struct {
	Steps                 []AcquisitionStepOutput
	CandidateRawDocuments []RawDocument
	AcceptedRawDocuments  []RawDocument
	Selections            []EvidenceSelectionDecision
	Canonicalization      CanonicalizationResult
	Corroboration         CorroborationResult
	Result                ResearchTaskResult
}

// AcquisitionPlanRunner executes every planned evidence channel. It is not a
// priority router and it does not stop after the first successful Retriever.
type AcquisitionPlanRunner struct {
	Now           func() time.Time
	SnapshotStore EvidenceSnapshotStore
}

func (r AcquisitionPlanRunner) Run(
	ctx context.Context,
	root ResearchTask,
	plan AcquisitionPlan,
) (AcquisitionPlanOutput, error) {
	root.ID = strings.TrimSpace(root.ID)
	root.Symbol = strings.ToUpper(strings.TrimSpace(root.Symbol))
	root.TargetSymbol = strings.ToUpper(
		strings.TrimSpace(root.TargetSymbol),
	)
	if root.TargetSymbol == "" {
		root.TargetSymbol = root.Symbol
	}
	if root.ID == "" {
		return AcquisitionPlanOutput{}, errors.New(
			"event root research task id is required",
		)
	}
	if root.Requirements.PITSafe && root.AsOf.IsZero() {
		return AcquisitionPlanOutput{}, errors.New(
			"event root research as_of is required for PIT-safe acquisition plan",
		)
	}
	if len(plan.Steps) == 0 {
		return AcquisitionPlanOutput{}, errors.New(
			"event acquisition plan requires at least one step",
		)
	}

	preparedSteps, err := prepareAcquisitionSteps(root, plan.Steps)
	if err != nil {
		return AcquisitionPlanOutput{}, err
	}

	now := r.Now
	if now == nil {
		now = time.Now
	}

	output := AcquisitionPlanOutput{
		Steps: make(
			[]AcquisitionStepOutput,
			0,
			len(preparedSteps),
		),
		CandidateRawDocuments: make(
			[]RawDocument,
			0,
		),
		AcceptedRawDocuments: make(
			[]RawDocument,
			0,
		),
		Selections: make(
			[]EvidenceSelectionDecision,
			0,
		),
	}

	attempts := make(
		[]AcquisitionAttempt,
		0,
		len(preparedSteps),
	)
	structuralErrors := make([]error, 0)

	for _, step := range preparedSteps {
		run, runErr := (TaskRunner{
			Now:           now,
			SnapshotStore: r.SnapshotStore,
		}).Run(
			ctx,
			step.Task,
			step.Retriever,
		)

		stepOutput := AcquisitionStepOutput{
			StepID: step.ID,
			Scope:  step.Scope,
			Task:   step.Task,
			Run:    run,
		}
		if runErr != nil {
			stepOutput.ErrorMessage = runErr.Error()
			structuralErrors = append(
				structuralErrors,
				fmt.Errorf(
					"acquisition step %q: %w",
					step.ID,
					runErr,
				),
			)
		}
		output.Steps = append(
			output.Steps,
			stepOutput,
		)

		if run.Attempt.ID != "" {
			attempts = append(
				attempts,
				run.Attempt,
			)
		}
		if runErr != nil {
			continue
		}

		usableDocuments := usableRawDocuments(run)
		switch step.Scope {
		case AcquisitionScopeDirect:
			output.AcceptedRawDocuments = append(
				output.AcceptedRawDocuments,
				usableDocuments...,
			)
		case AcquisitionScopeCandidate:
			output.CandidateRawDocuments = append(
				output.CandidateRawDocuments,
				usableDocuments...,
			)
		}
	}

	if len(structuralErrors) > 0 {
		return output, errors.Join(structuralErrors...)
	}

	if len(output.CandidateRawDocuments) > 0 {
		if plan.Selector == nil {
			return output, errors.New(
				"event evidence selector is required for candidate documents",
			)
		}

		decisions, selectErr := plan.Selector.Select(
			ctx,
			root,
			output.CandidateRawDocuments,
		)
		output.Selections = append(
			output.Selections,
			decisions...,
		)
		if selectErr != nil {
			return output, fmt.Errorf(
				"select candidate evidence: %w",
				selectErr,
			)
		}

		selected, validationErr := applyEvidenceSelection(
			output.CandidateRawDocuments,
			decisions,
		)
		if validationErr != nil {
			return output, validationErr
		}

		output.AcceptedRawDocuments = append(
			output.AcceptedRawDocuments,
			selected...,
		)
	}

	canonicalization, err := (Canonicalizer{}).Canonicalize(
		root,
		output.AcceptedRawDocuments,
	)
	if err != nil {
		return output, err
	}
	output.Canonicalization = canonicalization

	corroboration := (EvidenceCorroborator{}).Corroborate(
		canonicalization.Documents,
	)
	output.Corroboration = corroboration

	result := (TaskEvaluator{
		Corroboration: &corroboration,
	}).Evaluate(
		root,
		canonicalization.Documents,
		attempts,
	)
	result.CompletedAt = now().UTC()
	output.Result = result

	return output, nil
}

func prepareAcquisitionSteps(
	root ResearchTask,
	steps []AcquisitionStep,
) ([]AcquisitionStep, error) {
	prepared := make(
		[]AcquisitionStep,
		0,
		len(steps),
	)
	seenIDs := make(map[string]struct{}, len(steps))

	for _, step := range steps {
		step.ID = strings.TrimSpace(step.ID)
		if step.ID == "" {
			return nil, errors.New(
				"event acquisition step id is required",
			)
		}
		if _, exists := seenIDs[step.ID]; exists {
			return nil, fmt.Errorf(
				"event acquisition step id %q is duplicated",
				step.ID,
			)
		}
		seenIDs[step.ID] = struct{}{}

		switch step.Scope {
		case AcquisitionScopeDirect,
			AcquisitionScopeCandidate:
		default:
			return nil, fmt.Errorf(
				"event acquisition step %q has invalid scope %q",
				step.ID,
				step.Scope,
			)
		}

		if step.Retriever == nil {
			return nil, fmt.Errorf(
				"event acquisition step %q requires a retriever",
				step.ID,
			)
		}

		task, err := prepareAcquisitionStepTask(
			root,
			step.Task,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"event acquisition step %q: %w",
				step.ID,
				err,
			)
		}
		step.Task = task
		prepared = append(prepared, step)
	}

	return prepared, nil
}

func prepareAcquisitionStepTask(
	root ResearchTask,
	task ResearchTask,
) (ResearchTask, error) {
	taskID := strings.TrimSpace(task.ID)
	if taskID != "" && taskID != root.ID {
		return ResearchTask{}, fmt.Errorf(
			"step task id %q must match root task id %q",
			taskID,
			root.ID,
		)
	}
	task.ID = root.ID

	task.Symbol = strings.ToUpper(
		strings.TrimSpace(task.Symbol),
	)
	task.TargetSymbol = strings.ToUpper(
		strings.TrimSpace(task.TargetSymbol),
	)
	rootTarget := strings.ToUpper(
		strings.TrimSpace(root.TargetSymbol),
	)
	if rootTarget == "" {
		rootTarget = strings.ToUpper(
			strings.TrimSpace(root.Symbol),
		)
	}
	if task.TargetSymbol == "" {
		task.TargetSymbol = rootTarget
	} else if rootTarget != "" &&
		task.TargetSymbol != rootTarget {
		return ResearchTask{}, fmt.Errorf(
			"step target symbol %q must match root target symbol %q",
			task.TargetSymbol,
			rootTarget,
		)
	}

	if strings.TrimSpace(task.Objective) == "" {
		task.Objective = root.Objective
	}
	if task.AsOf.IsZero() {
		task.AsOf = root.AsOf
	}
	if task.TimeRange.Start.IsZero() {
		task.TimeRange.Start = root.TimeRange.Start
	}
	if task.TimeRange.End.IsZero() {
		task.TimeRange.End = root.TimeRange.End
	}

	if root.Requirements.PITSafe {
		if !task.AsOf.Equal(root.AsOf) {
			return ResearchTask{}, errors.New(
				"PIT-safe step task as_of must match root research as_of",
			)
		}
		task.Requirements.PITSafe = true
	}

	return task, nil
}

func usableRawDocuments(
	run TaskRunOutput,
) []RawDocument {
	usableIDs := make(map[string]struct{})
	for _, document := range run.Canonicalization.Documents {
		for _, rawID := range document.RawDocumentIDs {
			rawID = strings.TrimSpace(rawID)
			if rawID == "" {
				continue
			}
			usableIDs[rawID] = struct{}{}
		}
	}

	documents := make(
		[]RawDocument,
		0,
		len(usableIDs),
	)
	for _, raw := range run.RawDocuments {
		if _, ok := usableIDs[
			strings.TrimSpace(raw.ID),
		]; !ok {
			continue
		}
		documents = append(documents, raw)
	}
	return documents
}

func applyEvidenceSelection(
	candidates []RawDocument,
	decisions []EvidenceSelectionDecision,
) ([]RawDocument, error) {
	candidateIDs := make(map[string]struct{})
	for _, document := range candidates {
		documentID := strings.TrimSpace(document.ID)
		if documentID == "" {
			return nil, errors.New(
				"event candidate raw document id is required",
			)
		}
		candidateIDs[documentID] = struct{}{}
	}

	selectedIDs := make(map[string]bool)
	seenDecisions := make(map[string]struct{})

	for _, decision := range decisions {
		documentID := strings.TrimSpace(
			decision.RawDocumentID,
		)
		if documentID == "" {
			return nil, errors.New(
				"event selection decision raw document id is required",
			)
		}
		if _, ok := candidateIDs[documentID]; !ok {
			return nil, fmt.Errorf(
				"event selection decision references unknown raw document %q",
				documentID,
			)
		}
		if _, duplicate := seenDecisions[documentID]; duplicate {
			return nil, fmt.Errorf(
				"event selection decision for raw document %q is duplicated",
				documentID,
			)
		}

		seenDecisions[documentID] = struct{}{}
		selectedIDs[documentID] = decision.Selected
	}

	for documentID := range candidateIDs {
		if _, ok := seenDecisions[documentID]; ok {
			continue
		}
		return nil, fmt.Errorf(
			"event selection decision is missing for raw document %q",
			documentID,
		)
	}

	selected := make(
		[]RawDocument,
		0,
		len(candidates),
	)
	for _, document := range candidates {
		if !selectedIDs[
			strings.TrimSpace(document.ID),
		] {
			continue
		}
		selected = append(selected, document)
	}

	return selected, nil
}
