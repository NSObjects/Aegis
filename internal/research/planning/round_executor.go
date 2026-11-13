package planning

import (
	"Aegis/internal/capability/event"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/schema"
)

const (
	researchRoundStateSessionKey  = "AegisResearchRoundState"
	ResearchRoundResultSessionKey = "AegisResearchRoundResult"
)

type RootTaskResolver func(
	ctx context.Context,
	input []adk.Message,
) (event.ResearchTask, error)

type ResearchRoundAttempt struct {
	StepID       string                  `json:"step_id"`
	Retriever    string                  `json:"retriever"`
	Method       event.AcquisitionMethod `json:"method"`
	Status       event.AcquisitionStatus `json:"status"`
	ErrorMessage string                  `json:"error,omitempty"`
	DocumentCount int                    `json:"document_count"`
}

type ResearchRoundGap struct {
	Type         event.ResearchGapType   `json:"type"`
	Description  string                  `json:"description"`
	Required     int                     `json:"required,omitempty"`
	Observed     int                     `json:"observed,omitempty"`
	SourceType   event.SourceType        `json:"source_type,omitempty"`
	Authority    event.EvidenceAuthority `json:"authority,omitempty"`
	EvidenceKind event.EvidenceKind      `json:"evidence_kind,omitempty"`
}

type ResearchRoundResult struct {
	Round                      int                      `json:"round"`
	TaskID                     string                   `json:"task_id"`
	Status                     event.ResearchTaskStatus `json:"status"`
	DocumentCount              int                      `json:"document_count"`
	ClusterCount               int                      `json:"cluster_count"`
	IndependentSourceCount     int                      `json:"independent_source_count"`
	RoundAcceptedDocumentCount int                      `json:"round_accepted_document_count"`
	CandidateCount             int                      `json:"candidate_count"`
	SelectedCount              int                      `json:"selected_count"`
	AttemptCount               int                      `json:"attempt_count"`
	Attempts                   []ResearchRoundAttempt   `json:"attempts"`
	Gaps                       []ResearchRoundGap       `json:"gaps"`
}

type researchRoundState struct {
	Round                int
	RootTask             event.ResearchTask
	AcceptedRawDocuments []event.RawDocument
	Attempts             []event.AcquisitionAttempt
}

type ResearchRoundArtifact struct {
	Round            int
	RootTask         event.ResearchTask
	Canonicalization event.CanonicalizationResult
	Corroboration    event.CorroborationResult
	Result           event.ResearchTaskResult
	Attempts         []event.AcquisitionAttempt
}

type ResearchRoundObserver func(
	ctx context.Context,
	artifact ResearchRoundArtifact,
)

type ResearchRoundExecutorConfig struct {
	ResolveRootTask    RootTaskResolver
	Compiler           Compiler
	Runner             event.AcquisitionPlanRunner
	OnRoundCompleted   ResearchRoundObserver
}

type ResearchRoundExecutor struct {
	resolveRootTask   RootTaskResolver
	compiler          Compiler
	runner            event.AcquisitionPlanRunner
	onRoundCompleted  ResearchRoundObserver
}

var _ adk.Agent = (*ResearchRoundExecutor)(nil)

func init() {
	schema.RegisterName[*researchRoundState](
		"_aegis_research_round_state",
	)
}

func NewResearchRoundExecutor(
	cfg ResearchRoundExecutorConfig,
) (*ResearchRoundExecutor, error) {
	if cfg.ResolveRootTask == nil {
		return nil, errors.New(
			"research round root task resolver is required",
		)
	}

	return &ResearchRoundExecutor{
		resolveRootTask:  cfg.ResolveRootTask,
		compiler:         cfg.Compiler,
		runner:           cfg.Runner,
		onRoundCompleted: cfg.OnRoundCompleted,
	}, nil
}

func (e *ResearchRoundExecutor) Name(
	context.Context,
) string {
	return "research_round_executor"
}

func (e *ResearchRoundExecutor) Description(
	context.Context,
) string {
	return "executes one complete Aegis evidence-acquisition research round"
}

func (e *ResearchRoundExecutor) Run(
	ctx context.Context,
	_ *adk.AgentInput,
	_ ...adk.AgentRunOption,
) *adk.AsyncIterator[*adk.AgentEvent] {
	iterator, generator :=
		adk.NewAsyncIteratorPair[*adk.AgentEvent]()

	go func() {
		defer generator.Close()

		resultJSON, err := e.executeRound(ctx)
		if err != nil {
			generator.Send(
				&adk.AgentEvent{
					Err: err,
				},
			)
			return
		}

		generator.Send(
			adk.EventFromMessage(
				schema.AssistantMessage(
					resultJSON,
					nil,
				),
				nil,
				schema.Assistant,
				"",
			),
		)
	}()

	return iterator
}

func (e *ResearchRoundExecutor) executeRound(
	ctx context.Context,
) (string, error) {
	if e == nil || e.resolveRootTask == nil {
		return "", errors.New(
			"research round executor is not initialized",
		)
	}

	planValue, ok := adk.GetSessionValue(
		ctx,
		planexecute.PlanSessionKey,
	)
	if !ok {
		return "", errors.New(
			"research round plan is missing from Eino session",
		)
	}

	researchPlan, ok := planValue.(*ResearchPlan)
	if !ok || researchPlan == nil {
		return "", fmt.Errorf(
			"research round plan has unexpected type %T",
			planValue,
		)
	}

	state, err := e.roundState(ctx)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(state.RootTask.ID) == "" {
		userInputValue, exists := adk.GetSessionValue(
			ctx,
			planexecute.UserInputSessionKey,
		)
		if !exists {
			return "", errors.New(
				"research round user input is missing from Eino session",
			)
		}

		userInput, valid := userInputValue.([]adk.Message)
		if !valid {
			return "", fmt.Errorf(
				"research round user input has unexpected type %T",
				userInputValue,
			)
		}

		rootTask, resolveErr := e.resolveRootTask(
			ctx,
			userInput,
		)
		if resolveErr != nil {
			return "", fmt.Errorf(
				"resolve research root task: %w",
				resolveErr,
			)
		}
		rootTask.ID = strings.TrimSpace(rootTask.ID)
		if rootTask.ID == "" {
			return "", errors.New(
				"resolved research root task id is required",
			)
		}
		state.RootTask = rootTask
	}

	acquisitionPlan, err := e.compiler.Compile(
		state.RootTask,
		researchPlan,
	)
	if err != nil {
		return "", fmt.Errorf(
			"compile research plan: %w",
			err,
		)
	}

	roundOutput, err := e.runner.Run(
		ctx,
		state.RootTask,
		acquisitionPlan,
	)
	if err != nil {
		return "", fmt.Errorf(
			"execute research acquisition plan: %w",
			err,
		)
	}

	state.Round++
	state.AcceptedRawDocuments = append(
		state.AcceptedRawDocuments,
		stripRawMetadata(
			roundOutput.AcceptedRawDocuments,
		)...,
	)

	currentAttempts := collectRoundAttempts(
		roundOutput.Steps,
	)
	state.Attempts = append(
		state.Attempts,
		currentAttempts...,
	)

	canonicalization, err :=
		(event.Canonicalizer{}).Canonicalize(
			state.RootTask,
			state.AcceptedRawDocuments,
		)
	if err != nil {
		return "", fmt.Errorf(
			"canonicalize accumulated research evidence: %w",
			err,
		)
	}

	corroboration := (event.EvidenceCorroborator{}).Corroborate(
		canonicalization.Documents,
	)

	result := (event.TaskEvaluator{
		Corroboration: &corroboration,
	}).Evaluate(
		state.RootTask,
		canonicalization.Documents,
		state.Attempts,
	)

	roundResult := buildResearchRoundResult(
		state.Round,
		result,
		corroboration,
		roundOutput,
		currentAttempts,
	)

	if e.onRoundCompleted != nil {
		e.onRoundCompleted(
			ctx,
			ResearchRoundArtifact{
				Round:            state.Round,
				RootTask:         state.RootTask,
				Canonicalization: canonicalization,
				Corroboration:    corroboration,
				Result:           result,
				Attempts: append(
					[]event.AcquisitionAttempt(nil),
					state.Attempts...,
				),
			},
		)
	}

	encoded, err := json.Marshal(roundResult)
	if err != nil {
		return "", fmt.Errorf(
			"encode research round result: %w",
			err,
		)
	}
	resultJSON := string(encoded)

	adk.AddSessionValue(
		ctx,
		researchRoundStateSessionKey,
		state,
	)
	adk.AddSessionValue(
		ctx,
		ResearchRoundResultSessionKey,
		resultJSON,
	)
	adk.AddSessionValue(
		ctx,
		planexecute.ExecutedStepSessionKey,
		resultJSON,
	)

	return resultJSON, nil
}

func (e *ResearchRoundExecutor) roundState(
	ctx context.Context,
) (*researchRoundState, error) {
	value, ok := adk.GetSessionValue(
		ctx,
		researchRoundStateSessionKey,
	)
	if !ok {
		state := &researchRoundState{
			AcceptedRawDocuments: make(
				[]event.RawDocument,
				0,
			),
			Attempts: make(
				[]event.AcquisitionAttempt,
				0,
			),
		}
		adk.AddSessionValue(
			ctx,
			researchRoundStateSessionKey,
			state,
		)
		return state, nil
	}

	state, valid := value.(*researchRoundState)
	if !valid || state == nil {
		return nil, fmt.Errorf(
			"research round state has unexpected type %T",
			value,
		)
	}
	return state, nil
}

func collectRoundAttempts(
	steps []event.AcquisitionStepOutput,
) []event.AcquisitionAttempt {
	attempts := make(
		[]event.AcquisitionAttempt,
		0,
		len(steps),
	)
	for _, step := range steps {
		if strings.TrimSpace(step.Run.Attempt.ID) == "" {
			continue
		}
		attempts = append(
			attempts,
			step.Run.Attempt,
		)
	}
	return attempts
}

func stripRawMetadata(
	documents []event.RawDocument,
) []event.RawDocument {
	stripped := make(
		[]event.RawDocument,
		0,
		len(documents),
	)
	for _, document := range documents {
		document.RawMetadata = nil
		stripped = append(
			stripped,
			document,
		)
	}
	return stripped
}

func buildResearchRoundResult(
	round int,
	result event.ResearchTaskResult,
	corroboration event.CorroborationResult,
	output event.AcquisitionPlanOutput,
	currentAttempts []event.AcquisitionAttempt,
) ResearchRoundResult {
	attempts := make(
		[]ResearchRoundAttempt,
		0,
		len(output.Steps),
	)
	for _, step := range output.Steps {
		attempt := step.Run.Attempt
		if strings.TrimSpace(attempt.ID) == "" {
			continue
		}

		attempts = append(
			attempts,
			ResearchRoundAttempt{
				StepID:        step.StepID,
				Retriever:     attempt.Tool,
				Method:        attempt.Method,
				Status:        attempt.Status,
				ErrorMessage:  attempt.ErrorMessage,
				DocumentCount: len(attempt.DocumentRefs),
			},
		)
	}

	gaps := make(
		[]ResearchRoundGap,
		0,
		len(result.Gaps),
	)
	for _, gap := range result.Gaps {
		gaps = append(
			gaps,
			ResearchRoundGap{
				Type:        gap.Type,
				Description: gap.Description,
				Required:    gap.Required,
				Observed:    gap.Observed,
				SourceType:   gap.SourceType,
				Authority:    gap.Authority,
				EvidenceKind: gap.EvidenceKind,
			},
		)
	}

	selectedCount := 0
	for _, decision := range output.Selections {
		if decision.Selected {
			selectedCount++
		}
	}

	return ResearchRoundResult{
		Round:                      round,
		TaskID:                     result.TaskID,
		Status:                     result.Status,
		DocumentCount:              len(result.Documents),
		ClusterCount:               len(corroboration.Clusters),
		IndependentSourceCount:     corroboration.IndependentSourceCount,
		RoundAcceptedDocumentCount: len(output.AcceptedRawDocuments),
		CandidateCount:             len(output.CandidateRawDocuments),
		SelectedCount:              selectedCount,
		AttemptCount:               len(currentAttempts),
		Attempts:                   attempts,
		Gaps:                       gaps,
	}
}
