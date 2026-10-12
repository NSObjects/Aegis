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
			"研究意向为 MONITOR（持续观察）；不发起任何组合操作。"}, nil
	case tradeintent.StatusRejected:
		return policyResult{DispositionNoAction, ReasonRejectedUpstream,
			"研究意向已在上游被拒绝；不发起任何组合操作。"}, nil
	case tradeintent.StatusCandidate:
		switch assessment {
		case risk.AssessmentHighRisk:
			return policyResult{DispositionBlocked, ReasonHighRisk,
				"风险审查判定为 HIGH_RISK（高风险），候选研究被阻止进入后续批准流程。"}, nil
		case risk.AssessmentInsufficientContext:
			return policyResult{DispositionReviewRequired, ReasonRiskUncertain,
				"风险输入信息不足；必须补齐缺失事实并进行人工复核。"}, nil
		case risk.AssessmentCaution:
			return policyResult{DispositionReviewRequired, ReasonRiskCaution,
				"风险审查要求谨慎处理，须进一步复核。"}, nil
		case risk.AssessmentAcceptable:
			if hasObservedValuation {
				return policyResult{DispositionReviewRequired, ReasonResearchReviewRequired,
					"已经取得账户实际货币敞口数据，但研究结论仍需人工评估，不自动执行。"}, nil
			}
			return policyResult{DispositionReviewRequired, ReasonAccountDataAbsent,
				"风险审查未发现已证实的问题，但未提供账户现金和货币敞口信息；不授权执行。"}, nil
		}
	}
	return policyResult{}, fmt.Errorf("unsupported intent status %q or risk assessment %q", status, assessment)
}
