package event

type Status string

const (
	StatusNormal  Status = "NORMAL"
	StatusNoEvent Status = "NO_EVENT"
	StatusNoData  Status = "NO_DATA"
)

type Type string

const (
	TypeAnnouncement Type = "ANNOUNCEMENT"
	TypeNews         Type = "NEWS"
	TypePolicy       Type = "POLICY"
	TypeRegulatory   Type = "REGULATORY"
	TypeOther        Type = "OTHER"
)

type Impact string

const (
	ImpactPositive Impact = "POSITIVE"
	ImpactNegative Impact = "NEGATIVE"
	ImpactNeutral  Impact = "NEUTRAL"
	ImpactMixed    Impact = "MIXED"
	ImpactUnknown  Impact = "UNKNOWN"
)

type Materiality string

const (
	MaterialityLow    Materiality = "LOW"
	MaterialityMedium Materiality = "MEDIUM"
	MaterialityHigh   Materiality = "HIGH"
)

type Coverage struct {
	Announcements bool `json:"announcements"`
	News          bool `json:"news"`
	Policy        bool `json:"policy"`
}

type Item struct {
	Type           Type        `json:"type"`
	PublishedAt    string      `json:"published_at"`
	Source         string      `json:"source"`
	Title          string      `json:"title"`
	URL            string      `json:"url,omitempty"`
	Fact           string      `json:"fact"`
	Interpretation string      `json:"interpretation"`
	Impact         Impact      `json:"impact"`
	Materiality    Materiality `json:"materiality"`
}

type Context struct {
	Symbol        string `json:"symbol"`
	RequestedAsOf string `json:"requested_as_of"`
	WindowStart   string `json:"window_start"`
	WindowEnd     string `json:"window_end"`
	Status        Status `json:"status"`

	Coverage Coverage `json:"coverage"`
	Events   []Item   `json:"events"`

	Evidence   []string `json:"evidence"`
	Risk       []string `json:"risk"`
	Confidence float64  `json:"confidence"`
}
