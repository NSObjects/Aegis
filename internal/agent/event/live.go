package event

import (
	capevent "Aegis/internal/capability/event"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
)

type LiveAgentConfig struct {
	Analysis *Agent

	CaptureRunner capevent.AcquisitionPlanRunner

	// Singular fields remain compatible with existing callers. New provider
	// fan-out should use the plural slices; both forms are merged and validated.
	OfficialRetriever capevent.Retriever
	BreakingRetriever capevent.Retriever

	OfficialRetrievers []capevent.Retriever
	BreakingRetrievers []capevent.Retriever
}

type LiveAgent struct {
	analysis             *Agent
	captureRunner        capevent.AcquisitionPlanRunner
	officialRetrievers   []capevent.Retriever
	breakingRetrievers   []capevent.Retriever
}

func NewLiveAgent(
	cfg LiveAgentConfig,
) (*LiveAgent, error) {
	if cfg.Analysis == nil {
		return nil, errors.New(
			"live event analysis agent is required",
		)
	}
	if cfg.CaptureRunner.SnapshotStore == nil {
		return nil, errors.New(
			"live event capture snapshot store is required",
		)
	}

	official, err := resolveLiveRetrieverSet(
		"official",
		cfg.OfficialRetriever,
		cfg.OfficialRetrievers,
	)
	if err != nil {
		return nil, err
	}
	breaking, err := resolveLiveRetrieverSet(
		"breaking-news",
		cfg.BreakingRetriever,
		cfg.BreakingRetrievers,
	)
	if err != nil {
		return nil, err
	}

	return &LiveAgent{
		analysis:           cfg.Analysis,
		captureRunner:     cfg.CaptureRunner,
		officialRetrievers: official,
		breakingRetrievers: breaking,
	}, nil
}

func (a *LiveAgent) Analyze(
	ctx context.Context,
	input Input,
	opts ...adk.AgentRunOption,
) (*Context, error) {
	if a == nil ||
		a.analysis == nil {
		return nil, errors.New(
			"live event agent is not initialized",
		)
	}

	task, err := normalizeAnalystTask(
		input.Task,
	)
	if err != nil {
		return nil, err
	}

	// In live mode the caller's AsOf is the event-publication cutoff.
	// Evidence is captured after that request time, then the inner PIT-safe
	// research task is frozen at the capture completion time. TimeRange.End
	// preserves the original event cutoff so newly published events after the
	// request are not silently pulled into this run.
	publicationCutoff := task.AsOf
	if task.TimeRange.End.IsZero() ||
		task.TimeRange.End.After(
			publicationCutoff,
		) {
		task.TimeRange.End =
			publicationCutoff
	}

	captureRoot := task
	captureRoot.Requirements =
		capevent.ResearchRequirements{
			PITSafe: false,
		}

	capturePlan := capevent.AcquisitionPlan{
		Steps: make(
			[]capevent.AcquisitionStep,
			0,
			len(a.officialRetrievers)+
				len(a.breakingRetrievers),
		),
	}
	capturePlan.Steps = append(
		capturePlan.Steps,
		liveCaptureSteps(
			"capture_official",
			task.Symbol,
			capevent.NeedOfficialDisclosure,
			a.officialRetrievers,
		)...,
	)
	capturePlan.Steps = append(
		capturePlan.Steps,
		liveCaptureSteps(
			"capture_breaking_news",
			"",
			capevent.NeedBreakingNews,
			a.breakingRetrievers,
		)...,
	)

	capture, err := a.captureRunner.Run(
		ctx,
		captureRoot,
		capturePlan,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"capture live event evidence: %w",
			err,
		)
	}

	capturedAt := capture.Result.CompletedAt
	if capturedAt.IsZero() {
		capturedAt = latestCaptureFinishedAt(
			capture,
		)
	}
	if capturedAt.IsZero() {
		return nil, errors.New(
			"live event capture completed without a capture timestamp",
		)
	}

	analysisInput := input
	analysisInput.Task = task
	analysisInput.Task.AsOf =
		capturedAt.UTC()

	result, err := a.analysis.Analyze(
		ctx,
		analysisInput,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	// The inner PIT task runs at T1 so snapshots observed during live capture
	// are eligible. Preserve T0 as the caller-visible requested cutoff and
	// expose T1 separately as the evidence observation cutoff.
	result.EvidenceAsOf = result.RequestedAsOf
	result.RequestedAsOf =
		publicationCutoff.UTC().Format(
			time.RFC3339,
		)

	for _, step := range capture.Steps {
		status := step.Run.Attempt.Status
		switch status {
		case capevent.AcquisitionSucceeded:
			continue
		case capevent.AcquisitionPartial,
			capevent.AcquisitionFailed,
			capevent.AcquisitionTimeout:
			risk := fmt.Sprintf(
				"live evidence capture %s ended with status %s",
				step.StepID,
				status,
			)
			if message := strings.TrimSpace(
				step.Run.Attempt.ErrorMessage,
			); message != "" {
				risk += ": " + message
			}
			result.Risk = appendUniqueText(
				result.Risk,
				risk,
			)
		}
	}

	if err := Validate(result); err != nil {
		return nil, fmt.Errorf(
			"validate live event context: %w",
			err,
		)
	}
	return result, nil
}

func resolveLiveRetrieverSet(
	channel string,
	single capevent.Retriever,
	providers []capevent.Retriever,
) ([]capevent.Retriever, error) {
	retrievers := make(
		[]capevent.Retriever,
		0,
		len(providers)+1,
	)
	if single != nil {
		retrievers = append(
			retrievers,
			single,
		)
	}
	retrievers = append(
		retrievers,
		providers...,
	)

	if len(retrievers) == 0 {
		return nil, fmt.Errorf(
			"live event %s retrievers are required",
			channel,
		)
	}

	validated := make(
		[]capevent.Retriever,
		0,
		len(retrievers),
	)
	seenIDs := make(
		map[string]struct{},
		len(retrievers),
	)

	for index, retriever := range retrievers {
		if retriever == nil {
			return nil, fmt.Errorf(
				"live event %s retriever %d is nil",
				channel,
				index,
			)
		}

		retrieverID := strings.TrimSpace(
			retriever.ID(),
		)
		if retrieverID == "" {
			return nil, fmt.Errorf(
				"live event %s retriever %d id is required",
				channel,
				index,
			)
		}
		if retriever.Method() == "" {
			return nil, fmt.Errorf(
				"live event %s retriever %q acquisition method is required",
				channel,
				retrieverID,
			)
		}
		if _, duplicate := seenIDs[retrieverID]; duplicate {
			return nil, fmt.Errorf(
				"live event %s retriever id %q is duplicated",
				channel,
				retrieverID,
			)
		}
		seenIDs[retrieverID] = struct{}{}
		validated = append(
			validated,
			retriever,
		)
	}

	return validated, nil
}

func liveCaptureSteps(
	baseID string,
	symbol string,
	need capevent.InformationNeed,
	retrievers []capevent.Retriever,
) []capevent.AcquisitionStep {
	steps := make(
		[]capevent.AcquisitionStep,
		0,
		len(retrievers),
	)

	for _, retriever := range retrievers {
		stepID := baseID
		if len(retrievers) > 1 {
			stepID += "@" +
				strings.TrimSpace(
					retriever.ID(),
				)
		}

		steps = append(
			steps,
			capevent.AcquisitionStep{
				ID: stepID,
				Scope: capevent.
					AcquisitionScopeDirect,
				Task: capevent.ResearchTask{
					Symbol:          symbol,
					InformationNeed: need,
				},
				Retriever: retriever,
			},
		)
	}

	return steps
}

func latestCaptureFinishedAt(
	output capevent.AcquisitionPlanOutput,
) time.Time {
	var latest time.Time
	for _, step := range output.Steps {
		finished := step.Run.Attempt.FinishedAt
		if finished.After(latest) {
			latest = finished
		}
	}
	return latest
}

func appendUniqueText(
	values []string,
	value string,
) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
}
