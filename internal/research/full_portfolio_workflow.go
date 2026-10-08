package research

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/research/synthesis"
	"context"
	"errors"
	"fmt"
)

// FullPortfolioWorkflow runs a single upstream research graph and hands the
// exact validated artifact into the independent Debate/Trading/Risk/Portfolio
// loop. No second research pass and no test-only account data.
type FullPortfolioWorkflow struct {
	Research *Workflow
	Review   *PortfolioReview
}

type FullPortfolioResult struct {
	ResearchArtifact *synthesis.Artifact `json:"research_artifact"`
	PortfolioReview  *PortfolioReviewResult `json:"portfolio_review"`
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
	// Reject invalid/unknown account input before any paid or live research.
	if err := portfolio.Validate(book); err != nil {
		return nil, fmt.Errorf("invalid portfolio context: %w", err)
	}
	artifact, err := w.Research.RunArtifact(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("upstream research: %w", err)
	}
	result, err := w.Review.Run(ctx, artifact, book)
	if err != nil {
		return nil, fmt.Errorf("portfolio review: %w", err)
	}
	return &FullPortfolioResult{
		ResearchArtifact: artifact,
		PortfolioReview: result,
	}, nil
}
