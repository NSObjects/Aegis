package event

import "context"

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
	CoverageCovered CoverageStatus = "COVERED"
	CoverageEmpty   CoverageStatus = "EMPTY"
	CoverageFailed  CoverageStatus = "FAILED"
)

type RawItem struct {
	SourceID    string     `json:"source_id"`
	SourceName  string     `json:"source_name"`
	Kind        SourceKind `json:"kind"`
	PublishedAt string     `json:"published_at,omitempty"`
	Title       string     `json:"title"`
	URL         string     `json:"url,omitempty"`
	Snippet     string     `json:"snippet,omitempty"`
	Rank        *int       `json:"rank,omitempty"`
}

type Coverage struct {
	SourceID   string         `json:"source_id"`
	SourceName string         `json:"source_name"`
	Kind       SourceKind     `json:"kind"`
	Status     CoverageStatus `json:"status"`
	ItemCount  int            `json:"item_count"`
	Note       string         `json:"note,omitempty"`
}

type SourceResult struct {
	EntityName string    `json:"entity_name,omitempty"`
	Coverage   Coverage  `json:"coverage"`
	Items      []RawItem `json:"items"`
}

type Source interface {
	ID() string
	Name() string
	Kind() SourceKind

	Fetch(
		ctx context.Context,
		req Request,
	) (SourceResult, error)
}
