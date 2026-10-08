package portfoliodecision

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"strings"
	"testing"
)

func testIntent(status tradeintent.Status) *tradeintent.Intent {
	return &tradeintent.Intent{
		Symbol: "600519.SH",
		Thesis: "grounded thesis",
		Evidence: []string{"technical and market research"},
		Risk: []string{"market weakness"},
		Status: status,
		Confidence: 0.65,
	}
}

func testBook(state portfolio.State) *portfolio.Context {
	switch state {
	case portfolio.StateNotProvided:
		return &portfolio.Context{State: state}
	case portfolio.StateEmpty:
		return &portfolio.Context{State: state, AsOf: "2026-10-08T09:00:00+08:00"}
	case portfolio.StatePositions:
		return &portfolio.Context{
			State: state,
			AsOf: "2026-10-08T09:00:00+08:00",
			Positions: []portfolio.Position{{Symbol: "600519.SH", Quantity: 100}},
		}
	default:
		panic("invalid test state")
	}
}

func testRisk(assessment risk.Assessment) *risk.Context {
	return &risk.Context{
		Symbol: "600519.SH",
		Assessment: assessment,
		Summary: "reviewed the supplied typed inputs",
		Confidence: 0.7,
	}
}

func TestEvaluateDispositionMatrix(t *testing.T) {
	tests := []struct {
		name string
		status tradeintent.Status
		book portfolio.State
		assessment risk.Assessment
		disposition Disposition
		reason ReasonCode
	}{
		{"monitor", tradeintent.StatusMonitor, portfolio.StateEmpty, risk.AssessmentAcceptable, DispositionNoAction, ReasonMonitorOnly},
		{"rejected", tradeintent.StatusRejected, portfolio.StateEmpty, risk.AssessmentHighRisk, DispositionNoAction, ReasonRejectedUpstream},
		{"candidate high risk", tradeintent.StatusCandidate, portfolio.StatePositions, risk.AssessmentHighRisk, DispositionBlocked, ReasonHighRisk},
		{"candidate caution", tradeintent.StatusCandidate, portfolio.StatePositions, risk.AssessmentCaution, DispositionReviewRequired, ReasonRiskCaution},
		{"candidate missing portfolio", tradeintent.StatusCandidate, portfolio.StateNotProvided, risk.AssessmentInsufficientContext, DispositionReviewRequired, ReasonRiskUncertain},
		{"candidate insufficient observed data", tradeintent.StatusCandidate, portfolio.StateEmpty, risk.AssessmentInsufficientContext, DispositionReviewRequired, ReasonRiskUncertain},
		{"candidate acceptable but cash unknown", tradeintent.StatusCandidate, portfolio.StateEmpty, risk.AssessmentAcceptable, DispositionReviewRequired, ReasonAccountDataAbsent},
		{"candidate acceptable with positions", tradeintent.StatusCandidate, portfolio.StatePositions, risk.AssessmentAcceptable, DispositionReviewRequired, ReasonAccountDataAbsent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := testIntent(tt.status)
			book := testBook(tt.book)
			review := testRisk(tt.assessment)
			result, err := Evaluate(intent, book, review)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if result.Disposition != tt.disposition || result.ReasonCode != tt.reason {
				t.Fatalf("result disposition/reason = %s/%s, want %s/%s",
					result.Disposition, result.ReasonCode, tt.disposition, tt.reason)
			}
			if err := Validate(result, intent, book, review); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestEvaluateRejectsInconsistentUpstreamRisk(t *testing.T) {
	_, err := Evaluate(
		testIntent(tradeintent.StatusCandidate),
		testBook(portfolio.StateNotProvided),
		testRisk(risk.AssessmentAcceptable),
	)
	if err == nil || !strings.Contains(err.Error(), "INSUFFICIENT_CONTEXT") {
		t.Fatalf("error = %v", err)
	}
}

func TestEvaluateRejectsSymbolMismatch(t *testing.T) {
	review := testRisk(risk.AssessmentCaution)
	review.Symbol = "000001.SZ"
	_, err := Evaluate(
		testIntent(tradeintent.StatusCandidate),
		testBook(portfolio.StateEmpty),
		review,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v", err)
	}
}

func TestEvaluateRejectsInvalidInputs(t *testing.T) {
	_, err := Evaluate(nil, testBook(portfolio.StateEmpty), testRisk(risk.AssessmentCaution))
	if err == nil || !strings.Contains(err.Error(), "invalid trade intent") {
		t.Fatalf("error = %v", err)
	}
	_, err = Evaluate(testIntent(tradeintent.StatusCandidate), nil, testRisk(risk.AssessmentCaution))
	if err == nil || !strings.Contains(err.Error(), "invalid portfolio context") {
		t.Fatalf("error = %v", err)
	}
	_, err = Evaluate(testIntent(tradeintent.StatusCandidate), testBook(portfolio.StateEmpty), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid risk context") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsForgedPortfolioApproval(t *testing.T) {
	intent := testIntent(tradeintent.StatusCandidate)
	book := testBook(portfolio.StateEmpty)
	review := testRisk(risk.AssessmentAcceptable)
	decision, err := Evaluate(intent, book, review)
	if err != nil {
		t.Fatal(err)
	}
	decision.Disposition = Disposition("APPROVED")
	if err := Validate(decision, intent, book, review); err == nil {
		t.Fatal("forged approval must fail")
	}
}

func TestValidateRejectsChangedUpstreamState(t *testing.T) {
	intent := testIntent(tradeintent.StatusCandidate)
	book := testBook(portfolio.StatePositions)
	review := testRisk(risk.AssessmentHighRisk)
	decision, err := Evaluate(intent, book, review)
	if err != nil {
		t.Fatal(err)
	}
	decision.RiskAssessment = risk.AssessmentAcceptable
	if err := Validate(decision, intent, book, review); err == nil {
		t.Fatal("forged upstream risk assessment must fail")
	}
	decision.RiskAssessment = risk.AssessmentHighRisk
	decision.PortfolioState = portfolio.StateEmpty
	if err := Validate(decision, intent, book, review); err == nil {
		t.Fatal("forged portfolio state must fail")
	}
}

func TestValidateRejectsInventedSummary(t *testing.T) {
	intent := testIntent(tradeintent.StatusCandidate)
	book := testBook(portfolio.StateEmpty)
	review := testRisk(risk.AssessmentAcceptable)
	decision, err := Evaluate(intent, book, review)
	if err != nil {
		t.Fatal(err)
	}
	decision.Summary = "Portfolio has enough cash and is approved to trade."
	if err := Validate(decision, intent, book, review); err == nil {
		t.Fatal("invented summary must fail")
	}
}
