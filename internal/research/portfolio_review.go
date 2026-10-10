package research

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/portfoliodecision"
	"Aegis/internal/research/debate"
	"Aegis/internal/research/evidenceledger"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
)

// The workflow owns orchestration; each role owns its own reasoning or policy.
// The input is a validated ResearchArtifact, produced by the upstream research graph.
type portfolioDebateRunner interface {
	Run(context.Context, *synthesis.Artifact) (*debate.State, error)
}

type portfolioResearchManager interface {
	BuildPlan(context.Context, *synthesis.Artifact, *debate.State, ...model.Option) (*manager.ResearchPlan, error)
}

type portfolioTradingAgent interface {
	AnalyzeResearchPlanIntent(context.Context, *synthesis.Artifact, *manager.ResearchPlan, ...adk.AgentRunOption) (*tradeintent.Intent, error)
}

type portfolioRiskAnalyst interface {
	Review(context.Context, *tradeintent.Intent, *portfolio.Context, ...model.Option) (*risk.Context, error)
}

type portfolioManager interface {
	Review(context.Context, *tradeintent.Intent, *portfolio.Context, *risk.Context) (*portfoliodecision.Decision, error)
}

type PortfolioReviewConfig struct {
	Debate           portfolioDebateRunner
	ResearchManager  portfolioResearchManager
	TradingAgent     portfolioTradingAgent
	RiskAnalyst      portfolioRiskAnalyst
	PortfolioManager portfolioManager
}

type PortfolioReview struct {
	cfg PortfolioReviewConfig
}

type PortfolioReviewResult struct {
	Debate            *debate.State               `json:"debate_state"`
	ResearchPlan      *manager.ResearchPlan       `json:"research_plan"`
	// Grounded deterministic projection, not inferred from TradeIntent.Evidence.
	EvidenceLedger    *evidenceledger.Ledger      `json:"evidence_ledger"`
	TradeIntent       *tradeintent.Intent         `json:"trade_intent"`
	RiskContext       *risk.Context               `json:"risk_context"`
	PortfolioDecision *portfoliodecision.Decision `json:"portfolio_decision"`
	// Only populated for validated, caller-observed valuation. Nil means
	// existing exposure is unknown, not zero.
	ObservedExposure  *portfolio.Exposure          `json:"observed_exposure,omitempty"`
}

// NewPortfolioReview wires the V2.9 reasoning stage to the V2.10 risk/portfolio stage.
// No agent is constructed internally and no account data is synthesized.
func NewPortfolioReview(cfg PortfolioReviewConfig) (*PortfolioReview, error) {
	if cfg.Debate == nil {
		return nil, errors.New("portfolio review debate runtime is required")
	}
	if cfg.ResearchManager == nil {
		return nil, errors.New("portfolio review research manager is required")
	}
	if cfg.TradingAgent == nil {
		return nil, errors.New("portfolio review trading agent is required")
	}
	if cfg.RiskAnalyst == nil {
		return nil, errors.New("portfolio review risk analyst is required")
	}
	if cfg.PortfolioManager == nil {
		return nil, errors.New("portfolio review portfolio manager is required")
	}
	return &PortfolioReview{cfg: cfg}, nil
}

func (w *PortfolioReview) Run(
	ctx context.Context,
	artifact *synthesis.Artifact,
	book *portfolio.Context,
) (*PortfolioReviewResult, error) {
	if w == nil {
		return nil, errors.New("portfolio review workflow is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := synthesis.Validate(artifact); err != nil {
		return nil, fmt.Errorf("invalid research artifact: %w", err)
	}
	// nil is invalid: callers must explicitly choose NOT_PROVIDED, EMPTY, or POSITIONS.
	if err := portfolio.Validate(book); err != nil {
		return nil, fmt.Errorf("invalid portfolio context: %w", err)
	}
	observedExposure, err := portfolio.ComputeExposure(book)
	if err != nil {
		return nil, fmt.Errorf("compute portfolio exposure: %w", err)
	}
	state, err := w.cfg.Debate.Run(ctx, artifact)
	if err != nil {
		return nil, fmt.Errorf("research debate: %w", err)
	}
	if err := debate.ValidateState(state, artifact); err != nil {
		return nil, fmt.Errorf("invalid debate state: %w", err)
	}
	if len(state.Turns) == 0 {
		return nil, errors.New("research debate returned no turns")
	}

	plan, err := w.cfg.ResearchManager.BuildPlan(ctx, artifact, state)
	if err != nil {
		return nil, fmt.Errorf("research manager: %w", err)
	}
	if err := manager.Validate(plan, artifact, state); err != nil {
		return nil, fmt.Errorf("invalid research plan: %w", err)
	}
	ledger, err := evidenceledger.Build(artifact, state, plan)
	if err != nil {
		return nil, fmt.Errorf("research evidence ledger: %w", err)
	}
	// Downstream agents must not mutate already-validated research pointers
	// after the independently copied Ledger has been constructed.
	researchInputs := struct {
		Artifact *synthesis.Artifact
		Debate   *debate.State
		Plan     *manager.ResearchPlan
	}{artifact, state, plan}
	beforeTrading, err := json.Marshal(researchInputs)
	if err != nil {
		return nil, fmt.Errorf("snapshot validated research inputs: %w", err)
	}

	intent, err := w.cfg.TradingAgent.AnalyzeResearchPlanIntent(ctx, artifact, plan)
	if err != nil {
		return nil, fmt.Errorf("trading agent: %w", err)
	}
	afterTrading, err := json.Marshal(researchInputs)
	if err != nil {
		return nil, fmt.Errorf("snapshot research inputs after trading: %w", err)
	}
	if !bytes.Equal(beforeTrading, afterTrading) {
		return nil, errors.New("trading agent mutated validated research inputs after evidence ledger construction")
	}
	if err := tradeintent.Validate(intent); err != nil {
		return nil, fmt.Errorf("invalid trade intent: %w", err)
	}
	if intent.Symbol != artifact.Symbol {
		return nil, fmt.Errorf("trade intent symbol %q does not match artifact symbol %q", intent.Symbol, artifact.Symbol)
	}

	// Risk and Portfolio roles receive shared pointers. They must not modify
	// previously validated trade intent or caller-observed account state.
	decisionInputs := struct {
		Intent *tradeintent.Intent
		Book   *portfolio.Context
	}{intent, book}
	beforeReview, err := json.Marshal(decisionInputs)
	if err != nil {
		return nil, fmt.Errorf("snapshot validated decision inputs: %w", err)
	}
	assertUnchanged := func(stage string) error {
		after, err := json.Marshal(decisionInputs)
		if err != nil {
			return fmt.Errorf("snapshot decision inputs after %s: %w", stage, err)
		}
		if !bytes.Equal(beforeReview, after) {
			return fmt.Errorf("%s mutated validated trade intent or portfolio context", stage)
		}
		return nil
	}

	riskContext, err := w.cfg.RiskAnalyst.Review(ctx, intent, book)
	if unchangedErr := assertUnchanged("risk analyst"); unchangedErr != nil { return nil, unchangedErr }
	if err != nil {
		return nil, fmt.Errorf("risk analyst: %w", err)
	}
	if err := risk.Validate(riskContext, intent, book); err != nil {
		return nil, fmt.Errorf("invalid risk context: %w", err)
	}

	// Portfolio Manager must not rewrite a validated upstream risk assessment,
	// even when its edits would make the final deterministic check self-consistent.
	validatedRisk, err := json.Marshal(riskContext)
	if err != nil { return nil, fmt.Errorf("snapshot validated risk context: %w", err) }
	final, err := w.cfg.PortfolioManager.Review(ctx, intent, book, riskContext)
	if unchangedErr := assertUnchanged("portfolio manager"); unchangedErr != nil { return nil, unchangedErr }
	currentRisk, snapshotErr := json.Marshal(riskContext)
	if snapshotErr != nil { return nil, fmt.Errorf("snapshot risk context after portfolio manager: %w", snapshotErr) }
	if !bytes.Equal(validatedRisk, currentRisk) { return nil, errors.New("portfolio manager mutated validated risk context") }
	if err != nil {
		return nil, fmt.Errorf("portfolio manager: %w", err)
	}
	if err := portfoliodecision.Validate(final, intent, book, riskContext); err != nil {
		return nil, fmt.Errorf("invalid portfolio decision: %w", err)
	}

	return &PortfolioReviewResult{
		Debate: state,
		ResearchPlan: plan,
		EvidenceLedger: ledger,
		TradeIntent: intent,
		RiskContext: riskContext,
		PortfolioDecision: final,
		ObservedExposure: observedExposure,
	}, nil
}
