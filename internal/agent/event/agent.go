package event

import (
	capevent "Aegis/internal/capability/event"
	"Aegis/internal/research/planning"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type Input struct {
	Task  capevent.ResearchTask
	Query string
}

type Config struct {
	Planning planning.PlanExecuteConfig
	Analyzer Analyzer
}

type Agent struct {
	planning planning.PlanExecuteConfig
	analyzer Analyzer
}

func NewAgent(
	_ context.Context,
	cfg Config,
) (*Agent, error) {
	if cfg.Planning.PlannerModel == nil {
		return nil, errors.New(
			"event analyst planner model is required",
		)
	}
	if cfg.Planning.ReplannerModel == nil {
		return nil, errors.New(
			"event analyst replanner model is required",
		)
	}

	return &Agent{
		planning: cfg.Planning,
		analyzer: cfg.Analyzer,
	}, nil
}

func (a *Agent) Analyze(
	ctx context.Context,
	input Input,
	opts ...adk.AgentRunOption,
) (*Context, error) {
	if a == nil {
		return nil, errors.New(
			"event analyst is not initialized",
		)
	}

	task, err := normalizeAnalystTask(
		input.Task,
	)
	if err != nil {
		return nil, err
	}

	query, err := eventPlannerQuery(
		task,
		input.Query,
	)
	if err != nil {
		return nil, err
	}

	var latest *planning.ResearchRoundArtifact
	planCfg := a.planning
	planCfg.Executor.ResolveRootTask = func(
		context.Context,
		[]adk.Message,
	) (capevent.ResearchTask, error) {
		return task, nil
	}
	planCfg.Executor.OnRoundCompleted = func(
		_ context.Context,
		artifact planning.ResearchRoundArtifact,
	) {
		captured := cloneResearchRoundArtifact(
			artifact,
		)
		latest = &captured
	}

	planAgent, err := planning.NewPlanExecuteAgent(
		ctx,
		planCfg,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create event evidence research loop: %w",
			err,
		)
	}

	runner := adk.NewRunner(
		ctx,
		adk.RunnerConfig{
			Agent: planAgent,
		},
	)
	iterator := runner.Run(
		ctx,
		[]adk.Message{
			schema.UserMessage(query),
		},
		opts...,
	)

	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event == nil || event.Err == nil {
			continue
		}
		return nil, fmt.Errorf(
			"run event evidence research loop: %w",
			event.Err,
		)
	}

	if latest == nil {
		return nil, errors.New(
			"event evidence research loop completed without a research round artifact",
		)
	}
	if latest.RootTask.ID != task.ID {
		return nil, fmt.Errorf(
			"event evidence research returned task %q, want %q",
			latest.RootTask.ID,
			task.ID,
		)
	}

	result, err := a.analyzer.Analyze(
		ctx,
		AnalyzeInput{
			Task:           latest.RootTask,
			ResearchStatus: latest.Result.Status,
			Documents: latest.
				Canonicalization.
				Documents,
			Corroboration: latest.Corroboration,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"analyze researched event evidence: %w",
			err,
		)
	}
	if err := Validate(result); err != nil {
		return nil, fmt.Errorf(
			"validate event analyst context: %w",
			err,
		)
	}
	return result, nil
}

func normalizeAnalystTask(
	task capevent.ResearchTask,
) (capevent.ResearchTask, error) {
	task.ID = strings.TrimSpace(task.ID)
	task.Symbol = strings.ToUpper(
		strings.TrimSpace(task.Symbol),
	)
	task.Objective = strings.TrimSpace(
		task.Objective,
	)

	if task.ID == "" {
		return capevent.ResearchTask{}, errors.New(
			"event analyst research task id is required",
		)
	}
	if task.Symbol == "" {
		return capevent.ResearchTask{}, errors.New(
			"event analyst symbol is required",
		)
	}
	if task.Objective == "" {
		return capevent.ResearchTask{}, errors.New(
			"event analyst objective is required",
		)
	}
	if task.AsOf.IsZero() {
		return capevent.ResearchTask{}, errors.New(
			"event analyst as_of is required",
		)
	}
	if !task.Requirements.PITSafe {
		return capevent.ResearchTask{}, errors.New(
			"event analyst requires PIT-safe research",
		)
	}

	switch task.InformationNeed {
	case "":
		task.InformationNeed =
			capevent.NeedEventEvidence
	case capevent.NeedEventEvidence:
	default:
		return capevent.ResearchTask{}, fmt.Errorf(
			"event analyst requires information need %q, got %q",
			capevent.NeedEventEvidence,
			task.InformationNeed,
		)
	}

	return task, nil
}

type plannerTaskEnvelope struct {
	Task      plannerTaskView `json:"task"`
	UserQuery string          `json:"user_query,omitempty"`
}

type plannerTaskView struct {
	ID          string                    `json:"id"`
	Symbol      string                    `json:"symbol"`
	Objective   string                    `json:"objective"`
	AsOf        string                    `json:"as_of"`
	TimeStart   string                    `json:"time_start,omitempty"`
	TimeEnd     string                    `json:"time_end,omitempty"`
	Requirements capevent.ResearchRequirements `json:"requirements"`
}

func eventPlannerQuery(
	task capevent.ResearchTask,
	userQuery string,
) (string, error) {
	view := plannerTaskView{
		ID:           task.ID,
		Symbol:       task.Symbol,
		Objective:    task.Objective,
		AsOf:         task.AsOf.UTC().Format(timeRFC3339),
		Requirements: task.Requirements,
	}
	if !task.TimeRange.Start.IsZero() {
		view.TimeStart = task.TimeRange.Start.UTC().
			Format(timeRFC3339)
	}
	if !task.TimeRange.End.IsZero() {
		view.TimeEnd = task.TimeRange.End.UTC().
			Format(timeRFC3339)
	}

	payload, err := json.Marshal(
		plannerTaskEnvelope{
			Task:      view,
			UserQuery: strings.TrimSpace(userQuery),
		},
	)
	if err != nil {
		return "", fmt.Errorf(
			"encode event analyst planner input: %w",
			err,
		)
	}

	return "Plan the next evidence-acquisition round for this typed News/Event research task. The task itself is authoritative and must not be rewritten:\n" +
		string(payload), nil
}

func cloneResearchRoundArtifact(
	artifact planning.ResearchRoundArtifact,
) planning.ResearchRoundArtifact {
	cloned := artifact
	cloned.Canonicalization.Documents = append(
		[]capevent.CanonicalDocument(nil),
		artifact.Canonicalization.Documents...,
	)
	cloned.Canonicalization.Rejected = append(
		[]capevent.RejectedDocument(nil),
		artifact.Canonicalization.Rejected...,
	)
	cloned.Corroboration.Relations = append(
		[]capevent.EvidenceRelation(nil),
		artifact.Corroboration.Relations...,
	)
	cloned.Corroboration.Clusters = append(
		[]capevent.EvidenceCluster(nil),
		artifact.Corroboration.Clusters...,
	)
	for index := range cloned.Corroboration.Clusters {
		cloned.Corroboration.Clusters[index].DocumentIDs =
			append(
				[]string(nil),
				artifact.Corroboration.Clusters[index].DocumentIDs...,
			)
		cloned.Corroboration.Clusters[index].SourceKeys =
			append(
				[]string(nil),
				artifact.Corroboration.Clusters[index].SourceKeys...,
			)
	}
	cloned.Result.Documents = append(
		[]string(nil),
		artifact.Result.Documents...,
	)
	cloned.Result.Gaps = append(
		[]capevent.ResearchGap(nil),
		artifact.Result.Gaps...,
	)
	cloned.Attempts = append(
		[]capevent.AcquisitionAttempt(nil),
		artifact.Attempts...,
	)
	return cloned
}
