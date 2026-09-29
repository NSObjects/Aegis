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

	CaptureRunner       capevent.AcquisitionPlanRunner
	OfficialRetriever   capevent.Retriever
	BreakingRetriever   capevent.Retriever
}

type LiveAgent struct {
	analysis            *Agent
	captureRunner       capevent.AcquisitionPlanRunner
	officialRetriever   capevent.Retriever
	breakingRetriever   capevent.Retriever
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
	if cfg.OfficialRetriever == nil {
		return nil, errors.New(
			"live event official retriever is required",
		)
	}
	if cfg.BreakingRetriever == nil {
		return nil, errors.New(
			"live event breaking-news retriever is required",
		)
	}

	return &LiveAgent{
		analysis:          cfg.Analysis,
		captureRunner:     cfg.CaptureRunner,
		officialRetriever: cfg.OfficialRetriever,
		breakingRetriever: cfg.BreakingRetriever,
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

	capture, err := a.captureRunner.Run(
		ctx,
		captureRoot,
		capevent.AcquisitionPlan{
			Steps: []capevent.AcquisitionStep{
				{
					ID: "capture_official",
					Scope: capevent.
						AcquisitionScopeDirect,
					Task: capevent.ResearchTask{
						Symbol: task.Symbol,
						InformationNeed: capevent.
							NeedOfficialDisclosure,
					},
					Retriever:
						a.officialRetriever,
				},
				{
					ID: "capture_breaking_news",
					Scope: capevent.
						AcquisitionScopeDirect,
					Task: capevent.ResearchTask{
						Symbol: "",
						InformationNeed: capevent.
							NeedBreakingNews,
					},
					Retriever:
						a.breakingRetriever,
				},
			},
		},
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
