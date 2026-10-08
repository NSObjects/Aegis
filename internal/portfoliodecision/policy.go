package portfoliodecision

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"errors"
	"fmt"
)

type policyResult struct {
	disposition Disposition
	reason      ReasonCode
	summary     string
}

// Evaluate is deterministic and fail-closed. It never authorizes execution.
// Neither observed valuation nor the current limit diagnostics proves a
// sized, executable trade or verified authorization. ACCEPTABLE never means approved.
func Evaluate(intent *tradeintent.Intent, book *portfolio.Context, review *risk.Context) (*Decision, error) {
	if err := validateInputs(intent, book, review); err != nil {
		return nil, err
	}
	outcome, err := classify(intent.Status, review.Assessment, book.Valuation != nil)
	if err != nil {
		return nil, err
	}
	result := &Decision{
		Symbol:         intent.Symbol,
		Disposition:    outcome.disposition,
		ReasonCode:     outcome.reason,
		Summary:        outcome.summary,
		PortfolioState: book.State,
		RiskAssessment: review.Assessment,
	}
	if err := Validate(result, intent, book, review); err != nil {
		return nil, err
	}
	return result, nil
}

func Validate(result *Decision, intent *tradeintent.Intent, book *portfolio.Context, review *risk.Context) error {
	if result == nil {
		return errors.New("portfolio decision is nil")
	}
	if err := validateInputs(intent, book, review); err != nil {
		return err
	}
	expected, err := classify(intent.Status, review.Assessment, book.Valuation != nil)
	if err != nil {
		return err
	}
	if result.Symbol != intent.Symbol {
		return fmt.Errorf("portfolio decision symbol %q does not match intent symbol %q", result.Symbol, intent.Symbol)
	}
	if result.PortfolioState != book.State {
		return fmt.Errorf("portfolio state %q does not match observed state %q", result.PortfolioState, book.State)
	}
	if result.RiskAssessment != review.Assessment {
		return fmt.Errorf("risk assessment %q does not match observed assessment %q", result.RiskAssessment, review.Assessment)
	}
	if result.Disposition != expected.disposition || result.ReasonCode != expected.reason {
		return fmt.Errorf("portfolio disposition %q / reason %q violates policy for intent %q and risk %q",
			result.Disposition, result.ReasonCode, intent.Status, review.Assessment)
	}
	if result.Summary != expected.summary {
		return errors.New("portfolio decision summary is not the canonical policy summary")
	}
	return nil
}

func validateInputs(intent *tradeintent.Intent, book *portfolio.Context, review *risk.Context) error {
	if err := tradeintent.Validate(intent); err != nil {
		return fmt.Errorf("invalid trade intent: %w", err)
	}
	if err := portfolio.Validate(book); err != nil {
		return fmt.Errorf("invalid portfolio context: %w", err)
	}
	if err := risk.Validate(review, intent, book); err != nil {
		return fmt.Errorf("invalid risk context: %w", err)
	}
	return nil
}

func classify(status tradeintent.Status, assessment risk.Assessment, hasObservedValuation bool) (policyResult, error) {
	switch status {
	case tradeintent.StatusMonitor:
		return policyResult{DispositionNoAction, ReasonMonitorOnly,
			"Trade intent is MONITOR; no portfolio action is requested."}, nil
	case tradeintent.StatusRejected:
		return policyResult{DispositionNoAction, ReasonRejectedUpstream,
			"Trade intent was rejected upstream; no portfolio action is requested."}, nil
	case tradeintent.StatusCandidate:
		switch assessment {
		case risk.AssessmentHighRisk:
			return policyResult{DispositionBlocked, ReasonHighRisk,
				"Risk review found HIGH_RISK; candidate is blocked from further approval."}, nil
		case risk.AssessmentInsufficientContext:
			return policyResult{DispositionReviewRequired, ReasonRiskUncertain,
				"Risk inputs are insufficient; manual review and missing facts are required."}, nil
		case risk.AssessmentCaution:
			return policyResult{DispositionReviewRequired, ReasonRiskCaution,
				"Risk review requires caution; a further review is required."}, nil
		case risk.AssessmentAcceptable:
			if hasObservedValuation {
				return policyResult{DispositionReviewRequired, ReasonExecutionContextMissing,
					"Observed portfolio value is available, but no executable trade size, price or authorization has been assessed."}, nil
			}
			return policyResult{DispositionReviewRequired, ReasonAccountDataAbsent,
				"Risk review found no established concern, but portfolio cash and monetary exposure are not provided; execution is not authorized."}, nil
		}
	}
	return policyResult{}, fmt.Errorf("unsupported intent status %q or risk assessment %q", status, assessment)
}
