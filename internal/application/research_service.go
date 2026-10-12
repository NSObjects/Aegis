// Package application exposes the existing research workflows through a typed,
// transport-independent entry point. CLI, HTTP and future background workers
// must share this execution path rather than assembling a second agent graph.
package application

import (
	"context"
	"errors"
	"fmt"

	"Aegis/internal/decision"
	"Aegis/internal/portfolio"
	"Aegis/internal/research"
	"Aegis/internal/researchlang"
)

// ResearchMode selects which existing, independently validated Aegis workflow
// is invoked. It is not a trade/execution instruction.
type ResearchMode string

const (
	ModeDecision  ResearchMode = "DECISION"
	ModePortfolio ResearchMode = "PORTFOLIO_REVIEW"
)

// DecisionRunner is implemented by the existing legacy Research Workflow.
type DecisionRunner interface {
	Run(context.Context, research.Input) (*decision.Decision, error)
}

// PortfolioRunner is implemented by the existing FullPortfolioWorkflow.
// The account snapshot comes from the caller; none is invented by the service.
type PortfolioRunner interface {
	Run(context.Context, research.Input, *portfolio.Context) (*research.FullPortfolioResult, error)
}

// ResearchRequest deliberately uses the existing typed research and portfolio
// contracts. HTTP DTOs, file inputs and queue metadata belong to adapters.
type ResearchRequest struct {
	// IdempotencyKey belongs to the request adapter; never forward it to the LLM,
	// serialize it in a provider payload, or include it in a workflow artifact.
	IdempotencyKey string `json:"-"`
	// RerunOf is optional audit provenance, not LLM context or a request to replay old inputs.
	RerunOf string `json:"rerun_of,omitempty"`
	Mode      ResearchMode
	Input     research.Input
	Portfolio *portfolio.Context
}

// ResearchResponse keeps both possible outputs typed while preserving the
// legacy CLI JSON root shape via Output(). Only one field is populated.
type ResearchResponse struct {
	Mode      ResearchMode
	Decision  *decision.Decision
	Portfolio *research.FullPortfolioResult
}

// Output returns the existing workflow result, not a new wrapper JSON schema.
// It is meant for the CLI adapter; HTTP adapters should use typed fields.
func (r *ResearchResponse) Output() any {
	if r == nil {
		return nil
	}
	switch r.Mode {
	case ModeDecision:
		return r.Decision
	case ModePortfolio:
		return r.Portfolio
	default:
		return nil
	}
}

// ResearchService coordinates one invocation without logging or mutating the
// research and risk contracts. Dependencies can be supplied per deployment.
type ResearchService struct {
	decision  DecisionRunner
	portfolio PortfolioRunner
}

func NewResearchService(decisionRunner DecisionRunner, portfolioRunner PortfolioRunner) (*ResearchService, error) {
	if decisionRunner == nil && portfolioRunner == nil {
		return nil, errors.New("research service requires at least one workflow runner")
	}
	return &ResearchService{decision: decisionRunner, portfolio: portfolioRunner}, nil
}

// Run is synchronous by design. Task IDs, durable state and async scheduling
// belong to the upcoming RunService, not to an untracked goroutine here.
func (s *ResearchService) Run(ctx context.Context, req ResearchRequest) (*ResearchResponse, error) {
	if s == nil {
		return nil, errors.New("research service is nil")
	}
	if ctx == nil {
		return nil, errors.New("research context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Scope language checks to NEW research executions. Existing artifacts and
	// standalone agent calls remain unchanged. This does not translate inputs.
	ctx = researchlang.RequireChinese(ctx)

	switch req.Mode {
	case ModeDecision:
		if req.Portfolio != nil {
			return nil, errors.New("decision research cannot accept a portfolio snapshot")
		}
		if s.decision == nil {
			return nil, errors.New("decision workflow is unavailable")
		}
		result, err := s.decision.Run(ctx, req.Input)
		if err != nil {
			return nil, fmt.Errorf("decision research: %w", err)
		}
		if result == nil {
			return nil, errors.New("decision research returned no result")
		}
		return &ResearchResponse{Mode: ModeDecision, Decision: result}, nil
	case ModePortfolio:
		if req.Portfolio == nil {
			// An absent account is not a known EMPTY account. Callers may
			// explicitly pass {State: NOT_PROVIDED} when they do not know it.
			return nil, errors.New("portfolio research requires an explicit account snapshot")
		}
		if s.portfolio == nil {
			return nil, errors.New("portfolio review workflow is unavailable")
		}
		result, err := s.portfolio.Run(ctx, req.Input, req.Portfolio)
		if err != nil {
			return nil, fmt.Errorf("portfolio research: %w", err)
		}
		if result == nil {
			return nil, errors.New("portfolio research returned no result")
		}
		return &ResearchResponse{Mode: ModePortfolio, Portfolio: result}, nil
	default:
		return nil, fmt.Errorf("unsupported research mode %q", req.Mode)
	}
}
