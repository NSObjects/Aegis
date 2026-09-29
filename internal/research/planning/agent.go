package planning

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/components/model"
)

type PlanExecuteConfig struct {
	PlannerModel   model.ToolCallingChatModel
	ReplannerModel model.ToolCallingChatModel

	Executor ResearchRoundExecutorConfig

	MaxIterations int
}

func NewPlanExecuteAgent(
	ctx context.Context,
	cfg PlanExecuteConfig,
) (adk.ResumableAgent, error) {
	if cfg.PlannerModel == nil {
		return nil, errors.New(
			"research plan-execute planner model is required",
		)
	}
	if cfg.ReplannerModel == nil {
		return nil, errors.New(
			"research plan-execute replanner model is required",
		)
	}

	planner, err := NewPlanner(
		ctx,
		cfg.PlannerModel,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create research planner: %w",
			err,
		)
	}

	executor, err := NewResearchRoundExecutor(
		cfg.Executor,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create research round executor: %w",
			err,
		)
	}

	replanner, err := NewReplanner(
		ctx,
		cfg.ReplannerModel,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create research replanner: %w",
			err,
		)
	}

	agent, err := planexecute.New(
		ctx,
		&planexecute.Config{
			Planner:       planner,
			Executor:      executor,
			Replanner:     replanner,
			MaxIterations: cfg.MaxIterations,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create Eino research plan-execute agent: %w",
			err,
		)
	}

	return agent, nil
}
