package event

import "time"

type InformationNeed string

const (
	NeedOfficialDisclosure InformationNeed = "official_disclosure"
)

type TimeRange struct {
	Start time.Time
	End   time.Time
}

type ResearchTask struct {
	ID              string
	Symbol          string
	Objective       string
	InformationNeed InformationNeed
	AsOf            time.Time
	TimeRange       TimeRange
	Requirements    ResearchRequirements
}

type ResearchRequirements struct {
	PITSafe               bool
	RequiredSourceTypes   []SourceType
	PreferredSourceTypes  []SourceType
	MinDocuments          int
	MinIndependentSources int
	RequirePrimarySource  bool
}

type SourceType string

const (
	SourceOfficial        SourceType = "official"
	SourceMainstreamMedia SourceType = "mainstream_media"
	SourceIndustryMedia   SourceType = "industry_media"
	SourceResearch        SourceType = "research"
	SourceSocial          SourceType = "social"
	SourceOther           SourceType = "other"
)

type AcquisitionMethod string

const (
	AcquisitionAPI       AcquisitionMethod = "api"
	AcquisitionWebSearch AcquisitionMethod = "web_search"
	AcquisitionCrawler   AcquisitionMethod = "crawler"
	AcquisitionMCP       AcquisitionMethod = "mcp"
)

type AcquisitionStatus string

const (
	AcquisitionSucceeded AcquisitionStatus = "succeeded"
	AcquisitionFailed    AcquisitionStatus = "failed"
	AcquisitionTimeout   AcquisitionStatus = "timeout"
)

type AcquisitionAttempt struct {
	ID           string
	TaskID       string
	Method       AcquisitionMethod
	Tool         string
	StartedAt    time.Time
	FinishedAt   time.Time
	Status       AcquisitionStatus
	ErrorMessage string
	DocumentRefs []string
}

type RawDocument struct {
	ID        string
	TaskID    string
	AttemptID string

	Title   string
	Content string
	URL     string

	Publisher string
	Author    string

	SourceType SourceType
	IsPrimary  bool

	PublishedAt *time.Time
	RetrievedAt time.Time

	RawMetadata map[string]any
}

type DocumentSource struct {
	Publisher  string
	URL        string
	SourceType SourceType
	IsPrimary  bool
}

type CanonicalDocument struct {
	ID string

	Title   string
	Content string

	Source      DocumentSource
	PublishedAt *time.Time
	ContentHash string

	RawDocumentIDs []string
	AttemptIDs     []string
}

type RejectionReason string

const (
	RejectionFuturePublishedAt  RejectionReason = "future_published_at"
	RejectionMissingPublishedAt RejectionReason = "missing_published_at"
)

type RejectedDocument struct {
	RawDocumentID string
	Reason        RejectionReason
}

type CanonicalizationResult struct {
	Documents []CanonicalDocument
	Rejected  []RejectedDocument
}

type ResearchTaskStatus string

const (
	TaskSatisfied   ResearchTaskStatus = "satisfied"
	TaskPartial     ResearchTaskStatus = "partial"
	TaskUnsatisfied ResearchTaskStatus = "unsatisfied"
)

type ResearchGapType string

const (
	GapNoUsableDocuments              ResearchGapType = "no_usable_documents"
	GapInsufficientDocuments          ResearchGapType = "insufficient_documents"
	GapInsufficientIndependentSources ResearchGapType = "insufficient_independent_sources"
	GapMissingRequiredSourceType      ResearchGapType = "missing_required_source_type"
	GapMissingPrimarySource           ResearchGapType = "missing_primary_source"
)

type ResearchGap struct {
	Type        ResearchGapType
	Description string
	Required    int
	Observed    int
	SourceType  SourceType
}

type ResearchTaskResult struct {
	TaskID     string
	Status     ResearchTaskStatus
	Documents  []string
	Attempts   []string
	Gaps       []ResearchGap
	CompletedAt time.Time
}
