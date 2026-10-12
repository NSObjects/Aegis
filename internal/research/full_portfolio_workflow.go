package research

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/portfoliodecision"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/research/semanticreview"
	"Aegis/internal/tradeintent"
	"context"
	"errors"
	"fmt"
	"time"
)

// semanticReviewRunner is intentionally a narrow optional dependency. The
// normal research/portfolio flow never constructs or calls a Reviewer.
type semanticReviewRunner interface {
	Review(context.Context,*semanticreview.Tasks)(*semanticreview.Report,error)
}

type SemanticReviewStatus string

const (
	SemanticReviewCompleted SemanticReviewStatus = "COMPLETED"
	SemanticReviewNoLinks SemanticReviewStatus = "NO_LINKS"
	SemanticReviewUnavailable SemanticReviewStatus = "UNAVAILABLE"
)

// SemanticReviewOutcome records optional advisory availability, not a new
// trading or risk decision. ReasonCode never contains raw provider output.
type SemanticReviewOutcome struct {
	Status SemanticReviewStatus `json:"status"`
	TaskCount int `json:"task_count"`
	Report *semanticreview.Report `json:"report,omitempty"`
	ReasonCode string `json:"reason_code,omitempty"`
}

// FullPortfolioWorkflow runs a single upstream research graph and hands the
// exact validated artifact into the independent Debate/Trading/Risk/Portfolio
// loop. No second research pass and no test-only account data.
type FullPortfolioWorkflow struct {
	Research *Workflow
	Review   *PortfolioReview
	// MaxPortfolioAge is a caller-defined freshness policy for observed
	// account snapshots. Zero preserves the previous no-age-limit behavior.
	MaxPortfolioAge time.Duration
	// Limits is an optional caller-authored CURRENT exposure policy. It is
	// not derived from the observed account and authorizes no orders.
	Limits *portfolio.Limits
	// Scenario is an optional, caller-authored non-executable trade what-if.
	Scenario *portfolio.TradeScenario
	// Explicit opt-in only. Runs after all core decisions have completed.
	SemanticReviewer semanticReviewRunner
}

// FinalRiskGate combines the independently validated current-limit diagnosis
// with the canonical non-executable PortfolioDecision. It never authorizes a trade.
// The original PortfolioDecision is preserved as an auditable input.
type FinalRiskGate struct {
	Disposition portfoliodecision.Disposition `json:"disposition"`
	Reason string `json:"reason"`
	PolicyID string `json:"policy_id,omitempty"`
	CurrentLimitsStatus portfolio.LimitReviewStatus `json:"current_limits_status,omitempty"`
}

func evaluateFinalRiskGate(decision *portfoliodecision.Decision, limits *portfolio.LimitReview) (*FinalRiskGate, error) {
	if decision == nil { return nil, errors.New("final risk gate requires portfolio decision") }
	gate := &FinalRiskGate{Disposition:decision.Disposition, Reason:"PORTFOLIO_DECISION"}
	if limits == nil { return gate,nil }
	gate.PolicyID=limits.PolicyID
	gate.CurrentLimitsStatus=limits.Status
	switch limits.Status {
	case portfolio.LimitReviewBreach:
		if decision.Disposition != portfoliodecision.DispositionNoAction {
			gate.Disposition=portfoliodecision.DispositionBlocked
			gate.Reason="CURRENT_LIMIT_BREACH"
		} else { gate.Reason="NO_ACTION_WITH_CURRENT_LIMIT_BREACH" }
	case portfolio.LimitReviewInsufficientData:
		if decision.Disposition != portfoliodecision.DispositionNoAction && decision.Disposition != portfoliodecision.DispositionBlocked {
			gate.Disposition=portfoliodecision.DispositionReviewRequired
			gate.Reason="CURRENT_LIMITS_INSUFFICIENT_DATA"
		}
	case portfolio.LimitReviewPass:
		// Passing a current exposure rule never upgrades review to approval.
	default:
		return nil,fmt.Errorf("unknown current limits status %q",limits.Status)
	}
	return gate,nil
}

type FullPortfolioResult struct {
	ResearchArtifact *synthesis.Artifact `json:"research_artifact"`
	PortfolioReview  *PortfolioReviewResult `json:"portfolio_review"`
	// Absent unless explicit independent risk limits were provided.
	CurrentLimitsReview *portfolio.LimitReview `json:"current_limits_review,omitempty"`
	FinalRiskGate *FinalRiskGate `json:"final_risk_gate"`
	TradeScenarioReview *portfolio.ScenarioReview `json:"trade_scenario_review,omitempty"`
	// Absent under default configuration; never alters the main decisions.
	SemanticReview *SemanticReviewOutcome `json:"semantic_review,omitempty"`
}

func (w *FullPortfolioWorkflow) Run(
	ctx context.Context,
	input Input,
	book *portfolio.Context,
) (*FullPortfolioResult, error) {
	if w == nil || w.Research == nil || w.Review == nil {
		return nil, errors.New("full portfolio workflow requires research and review")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Validate the account's complete time contract before any paid or
	// external research. Use the requested research time, not time.Now().
	if _, err := normalizeWorkflowInput(input); err != nil {
		return nil, fmt.Errorf("invalid research input: %w", err)
	}
	if err := portfolio.ValidateAt(book, input.RequestedAsOf, w.MaxPortfolioAge); err != nil {
		return nil, fmt.Errorf("invalid portfolio snapshot: %w", err)
	}
	// Check the independently supplied policy at the same PIT cutoff as
	// research, before invoking any live model or external data source.
	limitsReview, err := portfolio.EvaluateCurrentLimits(book, w.Limits, input.RequestedAsOf)
	if err != nil {
		return nil, fmt.Errorf("current risk limits review: %w", err)
	}
	if err := portfolio.ValidateTradeScenarioAt(w.Scenario, book, w.Limits,
		input.RequestedAsOf, input.Symbol); err != nil {
		return nil, fmt.Errorf("invalid trade scenario: %w", err)
	}

	artifact, err := w.Research.RunArtifact(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("upstream research: %w", err)
	}
	result, err := w.Review.Run(ctx, artifact, book)
	if err != nil {
		return nil, fmt.Errorf("portfolio review: %w", err)
	}
	var scenarioReview *portfolio.ScenarioReview
	if w.Scenario != nil {
		if result.TradeIntent.Status != tradeintent.StatusCandidate {
			// MONITOR/REJECTED cannot be used as grounds for an executable
			// scenario. Do not even compute an indicative projection.
			scenarioReview = portfolio.BuildNotApplicableScenario(
				w.Scenario, w.Limits,
				fmt.Sprintf("研究意向为 %s，并非 CANDIDATE，因此没有计算交易后的敞口预测",
					result.TradeIntent.Status))
		} else {
			scenarioReview, err = portfolio.EvaluateTradeScenario(
				book, w.Limits, w.Scenario, input.RequestedAsOf, input.Symbol)
			if err != nil {
				return nil, fmt.Errorf("evaluate hypothetical trade scenario: %w", err)
			}
		}
	}
	finishGate := startProgress(ctx,"FinalRiskGate")
	gate, err := evaluateFinalRiskGate(result.PortfolioDecision, limitsReview)
	finishGate(&err)
	if err != nil { return nil, fmt.Errorf("final risk gate: %w",err) }
	fullResult := &FullPortfolioResult{
		ResearchArtifact: artifact,
		PortfolioReview: result,
		CurrentLimitsReview: limitsReview,
		FinalRiskGate: gate,
		TradeScenarioReview: scenarioReview,
	}
	// Purely advisory and explicitly enabled; performed AFTER all core stages.
	// Fail-open with typed availability instead of inventing a verdict or
	// dropping an already validated portfolio research result.
	if w.SemanticReviewer != nil {
		outcome := &SemanticReviewOutcome{Status: SemanticReviewUnavailable}
		fullResult.SemanticReview = outcome
		tasks, taskErr := semanticreview.BuildTasks(artifact, result.Debate, result.ResearchPlan)
		switch {
		case taskErr != nil:
			outcome.ReasonCode = "TASK_BUILD_ERROR"
		case len(tasks.Items) == 0:
			outcome.Status = SemanticReviewNoLinks
		default:
			outcome.TaskCount = len(tasks.Items)
			advisory, reviewErr := w.SemanticReviewer.Review(ctx, tasks)
			if reviewErr != nil {
				outcome.ReasonCode = "REVIEWER_ERROR"
			} else if err := semanticreview.ValidateReport(tasks, advisory); err != nil {
				outcome.ReasonCode = "INVALID_REPORT"
			} else {
				outcome.Status = SemanticReviewCompleted
				outcome.Report = advisory
			}
		}
	}
	return fullResult, nil
}
