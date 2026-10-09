package risk

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/tradeintent"
	"errors"
	"fmt"
	"math"
	"strings"
)

func Validate(ctx *Context, intent *tradeintent.Intent, portfolioCtx *portfolio.Context) error {
	if ctx == nil {
		return errors.New("risk context is nil")
	}
	if err := tradeintent.Validate(intent); err != nil {
		return fmt.Errorf("invalid trade intent: %w", err)
	}
	if err := portfolio.Validate(portfolioCtx); err != nil {
		return fmt.Errorf("invalid portfolio context: %w", err)
	}

	symbol := strings.TrimSpace(ctx.Symbol)
	if symbol == "" {
		return errors.New("risk context symbol is required")
	}
	if symbol != strings.TrimSpace(intent.Symbol) {
		return fmt.Errorf("risk context symbol %q does not match trade intent symbol %q", symbol, intent.Symbol)
	}

	switch ctx.Assessment {
	case AssessmentAcceptable, AssessmentCaution, AssessmentHighRisk, AssessmentInsufficientContext:
	default:
		return fmt.Errorf("invalid risk assessment %q", ctx.Assessment)
	}

	if strings.TrimSpace(ctx.Summary) == "" {
		return errors.New("risk context summary is required")
	}
	if math.IsNaN(ctx.Confidence) || ctx.Confidence < 0 || ctx.Confidence > 1 {
		return fmt.Errorf("confidence out of range: %f", ctx.Confidence)
	}

	if portfolioCtx.State == portfolio.StateNotProvided &&
		ctx.Assessment != AssessmentInsufficientContext {
		return errors.New("portfolio NOT_PROVIDED requires INSUFFICIENT_CONTEXT risk assessment")
	}

	for i, item := range ctx.Findings {
		if err := validateItem(item, intent, portfolioCtx); err != nil {
			return fmt.Errorf("finding %d: %w", i, err)
		}
	}
	for i, item := range ctx.Constraints {
		if err := validateItem(item, intent, portfolioCtx); err != nil {
			return fmt.Errorf("constraint %d: %w", i, err)
		}
	}
	for i, item := range ctx.Unresolved {
		if err := validateItem(item, intent, portfolioCtx); err != nil {
			return fmt.Errorf("unresolved %d: %w", i, err)
		}
	}
	return nil
}

func validateItem(item Item, intent *tradeintent.Intent, portfolioCtx *portfolio.Context) error {
	if strings.TrimSpace(item.Claim) == "" {
		return errors.New("claim is required")
	}
	if len(item.References) == 0 {
		return errors.New("at least one reference is required")
	}
	for i, ref := range item.References {
		if err := validateReference(ref, intent, portfolioCtx); err != nil {
			return fmt.Errorf("reference %d: %w", i, err)
		}
	}
	return nil
}

func validateReference(ref Reference, intent *tradeintent.Intent, portfolioCtx *portfolio.Context) error {
	switch ref.Domain {
	case ReferenceTradeIntent:
		if strings.TrimSpace(ref.Symbol) != "" &&
			strings.TrimSpace(ref.Symbol) != strings.TrimSpace(intent.Symbol) {
			return fmt.Errorf("trade intent reference symbol %q does not match %q", ref.Symbol, intent.Symbol)
		}
		return nil

	case ReferencePortfolio:
		if portfolioCtx.State == portfolio.StateNotProvided {
			return errors.New("portfolio reference is not allowed when portfolio is NOT_PROVIDED")
		}

		symbol := strings.TrimSpace(ref.Symbol)
		if symbol == "" {
			return nil
		}
		if portfolioCtx.State != portfolio.StatePositions {
			return fmt.Errorf("portfolio position reference %q requires POSITIONS state", symbol)
		}
		for _, position := range portfolioCtx.Positions {
			if strings.TrimSpace(position.Symbol) == symbol {
				return nil
			}
		}
		return fmt.Errorf("portfolio position %q does not exist", symbol)

	default:
		return fmt.Errorf("invalid reference domain %q", ref.Domain)
	}
}
