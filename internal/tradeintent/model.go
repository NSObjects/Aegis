package tradeintent

type Status string

const (
	StatusMonitor   Status = "MONITOR"
	StatusRejected  Status = "REJECTED"
	StatusCandidate Status = "CANDIDATE"
)

type Intent struct {
	Symbol     string   `json:"symbol"`
	Thesis     string   `json:"thesis"`
	Evidence   []string `json:"evidence"`
	Risk       []string `json:"risk"`
	Status     Status   `json:"status"`
	Confidence float64  `json:"confidence"`
}
