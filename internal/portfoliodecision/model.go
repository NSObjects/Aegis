package portfoliodecision

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/risk"
)

type Disposition string

const (
	DispositionNoAction       Disposition = "NO_ACTION"
	DispositionBlocked        Disposition = "BLOCKED"
	DispositionReviewRequired Disposition = "REVIEW_REQUIRED"
)

type ReasonCode string

const (
	ReasonMonitorOnly       ReasonCode = "MONITOR_ONLY"
	ReasonRejectedUpstream  ReasonCode = "REJECTED_UPSTREAM"
	ReasonHighRisk          ReasonCode = "HIGH_RISK"
	ReasonRiskUncertain     ReasonCode = "RISK_UNCERTAIN"
	ReasonRiskCaution       ReasonCode = "RISK_CAUTION"
	ReasonAccountDataAbsent ReasonCode = "ACCOUNT_DATA_ABSENT"
)

// Decision is a portfolio review disposition, NEVER an executable trade order.
type Decision struct {
	Symbol         string          `json:"symbol"`
	Disposition    Disposition     `json:"disposition"`
	ReasonCode     ReasonCode      `json:"reason_code"`
	Summary        string          `json:"summary"`
	PortfolioState portfolio.State `json:"portfolio_state"`
	RiskAssessment risk.Assessment `json:"risk_assessment"`
}
