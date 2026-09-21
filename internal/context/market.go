package context

type MarketContext struct {
	Indexes        []IndexSnapshot     `json:"indexes"`
	AdvanceCount   int                 `json:"advance_count"`
	DeclineCount   int                 `json:"decline_count"`
	LimitUpCount   int                 `json:"limit_up_count"`
	LimitDownCount int                 `json:"limit_down_count"`
	Turnover       float64             `json:"turnover"`
	Sectors        []SectorPerformance `json:"sectors"`
}

type IndexSnapshot struct {
	Name          string  `json:"name"`
	ChangePercent float64 `json:"change_percent"`
}

type SectorPerformance struct {
	Name          string  `json:"name"`
	ChangePercent float64 `json:"change_percent"`
}
