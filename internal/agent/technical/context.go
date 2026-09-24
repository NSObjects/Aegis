package technical

type Status string

const (
	StatusNormal    Status = "NORMAL"
	StatusSuspended Status = "SUSPENDED"
	StatusNotListed Status = "NOT_LISTED"
	StatusDelisted  Status = "DELISTED"
	StatusNoData    Status = "NO_DATA"
)

type Snapshot struct {
	Open          float64  `json:"open"`
	High          float64  `json:"high"`
	Low           float64  `json:"low"`
	Close         float64  `json:"close"`
	PreClose      float64  `json:"pre_close"`
	ChangePercent float64  `json:"change_percent"`
	Volume        float64  `json:"volume"`
	Amount        float64  `json:"amount"`
	TurnoverRate  *float64 `json:"turnover_rate,omitempty"`
}

type Returns struct {
	Return1D  *float64 `json:"return_1d,omitempty"`
	Return5D  *float64 `json:"return_5d,omitempty"`
	Return10D *float64 `json:"return_10d,omitempty"`
	Return20D *float64 `json:"return_20d,omitempty"`
	Return60D *float64 `json:"return_60d,omitempty"`
}

type MovingAverages struct {
	MA5  *float64 `json:"ma5,omitempty"`
	MA10 *float64 `json:"ma10,omitempty"`
	MA20 *float64 `json:"ma20,omitempty"`
	MA60 *float64 `json:"ma60,omitempty"`
}

type MACD struct {
	DIF  *float64 `json:"dif,omitempty"`
	DEA  *float64 `json:"dea,omitempty"`
	HIST *float64 `json:"hist,omitempty"`
}

type Momentum struct {
	RSI14 *float64 `json:"rsi14,omitempty"`
	MACD  MACD     `json:"macd"`
}

type VolumeMetrics struct {
	VolumeMA5   *float64 `json:"volume_ma5,omitempty"`
	VolumeMA20  *float64 `json:"volume_ma20,omitempty"`
	VolumeVsMA5 *float64 `json:"volume_vs_ma5,omitempty"`
	VolumeVsMA20 *float64 `json:"volume_vs_ma20,omitempty"`

	AmountMA5    *float64 `json:"amount_ma5,omitempty"`
	AmountMA20   *float64 `json:"amount_ma20,omitempty"`
	AmountVsMA5  *float64 `json:"amount_vs_ma5,omitempty"`
	AmountVsMA20 *float64 `json:"amount_vs_ma20,omitempty"`
}

type Context struct {
	Symbol          string `json:"symbol"`
	RequestedAsOf   string `json:"requested_as_of"`
	EffectiveAsOf   string `json:"effective_as_of,omitempty"`
	LastTradeDate   string `json:"last_trade_date,omitempty"`
	Status          Status `json:"status"`

	Snapshot       *Snapshot       `json:"snapshot,omitempty"`
	Returns        Returns         `json:"returns"`
	MovingAverages MovingAverages  `json:"moving_averages"`
	Momentum       Momentum        `json:"momentum"`
	Volume         VolumeMetrics   `json:"volume"`

	Evidence   []string `json:"evidence"`
	Risk       []string `json:"risk"`
	Confidence float64  `json:"confidence"`
}
