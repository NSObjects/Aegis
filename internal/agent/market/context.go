package market

type Regime string

const (
	RegimeRiskOn  Regime = "RISK_ON"
	RegimeMixed   Regime = "MIXED"
	RegimeRiskOff Regime = "RISK_OFF"
)

type IndexSnapshot struct {
	Name          string  `json:"name"`
	ChangePercent float64 `json:"change_percent"`
}

type SectorPerformance struct {
	Name          string  `json:"name"`
	ChangePercent float64 `json:"change_percent"`
}

type Context struct {
	AsOf string `json:"as_of"`

	Regime Regime `json:"regime"`

	Indexes []IndexSnapshot `json:"indexes"`

	AdvanceCount   int `json:"advance_count"`
	DeclineCount   int `json:"decline_count"`
	LimitUpCount   int `json:"limit_up_count"`
	LimitDownCount int `json:"limit_down_count"`

	Turnover float64 `json:"turnover"`

	LeadingSectors []SectorPerformance `json:"leading_sectors"`

	Evidence []string `json:"evidence"`
	Risk     []string `json:"risk"`

	Confidence float64 `json:"confidence"`
}
