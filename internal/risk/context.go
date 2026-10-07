package risk

type Assessment string

const (
	AssessmentAcceptable          Assessment = "ACCEPTABLE"
	AssessmentCaution             Assessment = "CAUTION"
	AssessmentHighRisk            Assessment = "HIGH_RISK"
	AssessmentInsufficientContext Assessment = "INSUFFICIENT_CONTEXT"
)

type ReferenceDomain string

const (
	ReferenceTradeIntent ReferenceDomain = "TRADE_INTENT"
	ReferencePortfolio   ReferenceDomain = "PORTFOLIO"
)

type Reference struct {
	Domain ReferenceDomain `json:"domain"`
	Symbol string          `json:"symbol,omitempty"`
}

type Item struct {
	Claim      string      `json:"claim"`
	References []Reference `json:"references"`
}

type Context struct {
	Symbol      string     `json:"symbol"`
	Assessment  Assessment `json:"assessment"`
	Summary     string     `json:"summary"`
	Findings    []Item     `json:"findings,omitempty"`
	Constraints []Item     `json:"constraints,omitempty"`
	Unresolved  []Item     `json:"unresolved,omitempty"`
	Confidence  float64    `json:"confidence"`
}
