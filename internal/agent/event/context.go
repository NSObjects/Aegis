package event

type Status string

const (
	StatusNormal  Status = "NORMAL"
	StatusNoEvent Status = "NO_EVENT"
	StatusPartial Status = "PARTIAL"
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

type SourceKind string

const (
	SourceOfficialDisclosure SourceKind = "OFFICIAL_DISCLOSURE"
	SourceFinancialNews       SourceKind = "FINANCIAL_NEWS"
	SourceGeneralNews         SourceKind = "GENERAL_NEWS"
	SourceSocialMedia         SourceKind = "SOCIAL_MEDIA"
	SourceWebSearch           SourceKind = "WEB_SEARCH"
)

type CoverageStatus string

const (
	CoverageCovered     CoverageStatus = "COVERED"
	CoverageEmpty       CoverageStatus = "EMPTY"
	CoverageFailed      CoverageStatus = "FAILED"
	CoverageUnavailable CoverageStatus = "UNAVAILABLE"
)

type SourceCoverage struct {
	SourceID   string         `json:"source_id"`
	SourceName string         `json:"source_name"`
	Kind       SourceKind     `json:"kind"`
	Status     CoverageStatus `json:"status"`
	ItemCount  int            `json:"item_count"`
	Note       string         `json:"note,omitempty"`
}

type SourceRef struct {
	SourceID    string `json:"source_id"`
	SourceName  string `json:"source_name"`
	PublishedAt string `json:"published_at,omitempty"`
	URL         string `json:"url,omitempty"`
}

type Item struct {
	Type           Type        `json:"type"`
	PublishedAt    string      `json:"published_at"`
	Title          string      `json:"title"`
	Fact           string      `json:"fact"`
	Interpretation string      `json:"interpretation"`
	Impact         Impact      `json:"impact"`
	Materiality    Materiality `json:"materiality"`
	Sources        []SourceRef `json:"sources"`
}

type Context struct {
	Symbol        string `json:"symbol"`
	RequestedAsOf string `json:"requested_as_of"`
	WindowStart   string `json:"window_start"`
	WindowEnd     string `json:"window_end"`
	Status        Status `json:"status"`

	Coverage []SourceCoverage `json:"coverage"`
	Events   []Item           `json:"events"`

	Evidence   []string `json:"evidence"`
	Risk       []string `json:"risk"`
	Confidence float64  `json:"confidence"`
}
