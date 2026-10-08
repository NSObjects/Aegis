package portfolioagent

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/portfoliodecision"
	"Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"context"
	"errors"
)

// Manager owns the final non-executable portfolio review boundary.
// No LLM is needed while the upstream account/risk-budget inputs are absent.
type Manager struct{}

func (m *Manager) Review(
	ctx context.Context,
	intent *tradeintent.Intent,
	book *portfolio.Context,
	review *risk.Context,
) (*portfoliodecision.Decision, error) {
	if m == nil {
		return nil, errors.New("portfolio manager is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return portfoliodecision.Evaluate(intent, book, review)
}
