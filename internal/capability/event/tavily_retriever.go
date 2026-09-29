package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	tavilySearchURL = "https://api.tavily.com/search"

	defaultTavilyMaxResults   = 5
	maxTavilyMaxResults       = 10
	defaultTavilyMaxTextChars = 40_000
	defaultTavilyLookback     = 7 * 24 * time.Hour
)

type TavilyWebSearchRetriever struct {
	Client         HTTPDoer
	Now            func() time.Time
	APIKey         string
	BaseURL        string
	TargetResolver EvidenceTargetResolver
	MaxResults     int
	MaxTextChars   int
}

func (TavilyWebSearchRetriever) ID() string {
	return "tavily_web_search"
}

func (TavilyWebSearchRetriever) Method() AcquisitionMethod {
	return AcquisitionWebSearch
}

func (r TavilyWebSearchRetriever) Retrieve(
	ctx context.Context,
	task ResearchTask,
) ([]RawDocument, error) {
	if task.InformationNeed != "" &&
		task.InformationNeed != NeedBreakingNews {
		return nil, fmt.Errorf(
			"tavily web search retriever does not support information need %q",
			task.InformationNeed,
		)
	}
	if strings.TrimSpace(task.Symbol) != "" {
		return nil, fmt.Errorf(
			"tavily web search retriever expects candidate task Symbol to be empty; got %q",
			task.Symbol,
		)
	}

	targetSymbol := strings.ToUpper(
		strings.TrimSpace(task.TargetSymbol),
	)
	if targetSymbol == "" {
		return nil, errors.New(
			"tavily web search retriever target symbol is required",
		)
	}

	target, err := r.resolveTarget(
		ctx,
		task,
		targetSymbol,
	)
	if err != nil {
		return nil, err
	}

	now := r.Now
	if now == nil {
		now = time.Now
	}
	retrievedAt := now().UTC()

	start, end, err := tavilyEffectiveQueryRange(
		task,
		retrievedAt,
	)
	if err != nil {
		return nil, err
	}

	maxResults := r.MaxResults
	if maxResults <= 0 {
		maxResults = defaultTavilyMaxResults
	}
	if maxResults > maxTavilyMaxResults {
		maxResults = maxTavilyMaxResults
	}

	maxTextChars := r.MaxTextChars
	if maxTextChars <= 0 {
		maxTextChars = defaultTavilyMaxTextChars
	}

	requestBody := tavilySearchRequest{
		Query:                 tavilyTargetQuery(target),
		SearchDepth:           "advanced",
		ChunksPerSource:       3,
		MaxResults:            maxResults,
		Topic:                 "news",
		StartDate:             tavilyDate(start),
		EndDate:               tavilyDate(end),
		IncludePublishedDate:  true,
		FilterByPublishedDate: false,
		IncludeAnswer:         false,
		IncludeRawContent:     "text",
		Language:              "zh-cn",
		FilterByLanguage:      false,
		SafeSearch:            true,
	}

	client := r.Client
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	payload, err := r.search(
		ctx,
		client,
		requestBody,
	)
	if err != nil {
		return nil, err
	}

	documents := make(
		[]RawDocument,
		0,
		len(payload.Results),
	)
	missingBody := 0
	missingDate := 0

	for _, result := range payload.Results {
		rawURL := strings.TrimSpace(result.URL)
		rawContent := normalizeTavilyText(
			result.RawContent,
		)
		if rawURL == "" || rawContent == "" {
			if rawURL != "" {
				missingBody++
			}
			continue
		}

		publishedAt := tavilyPublishedTime(
			result.PublishedDate,
		)
		if !end.IsZero() && publishedAt == nil {
			missingDate++
			continue
		}
		if !withinResearchTimeRange(
			publishedAt,
			TimeRange{
				Start: start,
				End:   end,
			},
		) {
			continue
		}

		content, truncated := truncateTextChars(
			rawContent,
			maxTextChars,
		)
		if strings.TrimSpace(content) == "" {
			missingBody++
			continue
		}

		documentURL := normalizeTavilyURL(
			rawURL,
		)
		if documentURL == "" {
			continue
		}

		document := RawDocument{
			ID: shortID(
				"tavily_web",
				documentURL,
			),
			Title:        normalizeTavilyText(result.Title),
			Content:      content,
			URL:          documentURL,
			Publisher:    tavilyPublisher(documentURL),
			SourceType:   SourceOther,
			EvidenceKind: EvidenceOther,
			IsPrimary:    false,
			PublishedAt:  publishedAt,
			FirstSeenAt:  cloneTime(&retrievedAt),
			AvailableAt:  cloneTime(&retrievedAt),
			RetrievedAt:  retrievedAt,
			RawMetadata: map[string]any{
				"provider":                "tavily",
				"result_id":               strings.TrimSpace(result.ID),
				"score":                   result.Score,
				"search_query":            requestBody.Query,
				"search_snippet":          normalizeTavilyText(result.Content),
				"published_date":          strings.TrimSpace(result.PublishedDate),
				"published_date_estimate": publishedAt != nil,
				"raw_content_source":      "tavily_search",
				"body_enriched":           true,
				"text_chars":              utf8.RuneCountInString(content),
				"body_truncated":          truncated,
			},
		}
		documents = append(
			documents,
			document,
		)
	}

	return documents, tavilyCoverageError(
		len(payload.Results),
		len(documents),
		missingBody,
		missingDate,
	)
}

func (r TavilyWebSearchRetriever) resolveTarget(
	ctx context.Context,
	task ResearchTask,
	targetSymbol string,
) (EvidenceTarget, error) {
	targetTask := task
	targetTask.Symbol = targetSymbol

	if r.TargetResolver == nil {
		return normalizeEvidenceTarget(
			targetTask,
			EvidenceTarget{
				Symbol: targetSymbol,
				Aliases: []string{
					snapshotBaseSymbol(targetSymbol),
				},
			},
		)
	}

	target, err := r.TargetResolver.Resolve(
		ctx,
		targetTask,
	)
	if err != nil {
		return EvidenceTarget{}, fmt.Errorf(
			"resolve tavily web search target: %w",
			err,
		)
	}
	target, err = normalizeEvidenceTarget(
		targetTask,
		target,
	)
	if err != nil {
		return EvidenceTarget{}, fmt.Errorf(
			"normalize tavily web search target: %w",
			err,
		)
	}
	return target, nil
}

type tavilySearchRequest struct {
	Query                 string `json:"query"`
	SearchDepth           string `json:"search_depth"`
	ChunksPerSource       int    `json:"chunks_per_source"`
	MaxResults            int    `json:"max_results"`
	Topic                 string `json:"topic"`
	StartDate             string `json:"start_date,omitempty"`
	EndDate               string `json:"end_date,omitempty"`
	IncludePublishedDate  bool   `json:"include_published_date"`
	FilterByPublishedDate bool   `json:"filter_by_published_date"`
	IncludeAnswer         bool   `json:"include_answer"`
	IncludeRawContent     string `json:"include_raw_content"`
	Language              string `json:"language"`
	FilterByLanguage      bool   `json:"filter_by_language"`
	SafeSearch            bool   `json:"safe_search"`
}

type tavilySearchResponse struct {
	Query     string               `json:"query"`
	Results   []tavilySearchResult `json:"results"`
	RequestID string               `json:"request_id"`
}

type tavilySearchResult struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Content       string  `json:"content"`
	RawContent    string  `json:"raw_content"`
	Score         float64 `json:"score"`
	PublishedDate string  `json:"published_date"`
}

func (r TavilyWebSearchRetriever) search(
	ctx context.Context,
	client HTTPDoer,
	body tavilySearchRequest,
) (tavilySearchResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return tavilySearchResponse{}, fmt.Errorf(
			"encode tavily search request: %w",
			err,
		)
	}

	endpoint := strings.TrimSpace(r.BaseURL)
	if endpoint == "" {
		endpoint = tavilySearchURL
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(data),
	)
	if err != nil {
		return tavilySearchResponse{}, fmt.Errorf(
			"build tavily search request: %w",
			err,
		)
	}
	req.Header.Set(
		"Content-Type",
		"application/json",
	)
	req.Header.Set(
		"Accept",
		"application/json",
	)

	apiKey := strings.TrimSpace(r.APIKey)
	if apiKey != "" {
		req.Header.Set(
			"Authorization",
			"Bearer "+apiKey,
		)
	} else {
		req.Header.Set(
			"X-Tavily-Access-Mode",
			"keyless",
		)
	}

	resp, err := client.Do(req)
	if err != nil {
		return tavilySearchResponse{}, fmt.Errorf(
			"request tavily web search: %w",
			err,
		)
	}
	defer resp.Body.Close()

	if err := requireHTTP2xx(
		resp,
		"tavily web search",
	); err != nil {
		return tavilySearchResponse{}, err
	}

	var payload tavilySearchResponse
	if err := json.NewDecoder(resp.Body).Decode(
		&payload,
	); err != nil {
		return tavilySearchResponse{}, fmt.Errorf(
			"decode tavily web search response: %w",
			err,
		)
	}
	return payload, nil
}

func tavilyTargetQuery(
	target EvidenceTarget,
) string {
	parts := make([]string, 0, 4)

	name := strings.TrimSpace(target.Name)
	if name != "" {
		name = strings.ReplaceAll(
			name,
			string(rune(34)),
			"",
		)
		parts = append(
			parts,
			string(rune(34))+name+string(rune(34)),
		)
	}

	baseSymbol := snapshotBaseSymbol(
		target.Symbol,
	)
	if baseSymbol != "" {
		parts = append(
			parts,
			baseSymbol,
		)
	}
	if len(parts) == 0 {
		for _, alias := range target.Aliases {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				parts = append(parts, alias)
				break
			}
		}
	}

	parts = append(
		parts,
		"最新 新闻 事件",
	)
	return strings.Join(parts, " ")
}

func tavilyEffectiveQueryRange(
	task ResearchTask,
	now time.Time,
) (time.Time, time.Time, error) {
	end := task.TimeRange.End
	if !task.AsOf.IsZero() &&
		(end.IsZero() || task.AsOf.Before(end)) {
		end = task.AsOf
	}
	if end.IsZero() {
		end = now
	}

	start := task.TimeRange.Start
	if start.IsZero() {
		start = end.Add(-defaultTavilyLookback)
	}
	if start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf(
			"tavily search time range start %s is after effective end %s",
			start.Format(time.RFC3339),
			end.Format(time.RFC3339),
		)
	}
	return start, end, nil
}

func tavilyDate(
	value time.Time,
) string {
	if value.IsZero() {
		return ""
	}
	return value.In(cninfoShanghaiLocation).
		Format("2006-01-02")
}

func tavilyPublishedTime(
	raw string,
) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	if parsed, err := http.ParseTime(raw); err == nil {
		value := parsed.UTC()
		return &value
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		parsed, err := time.Parse(
			layout,
			raw,
		)
		if err != nil {
			continue
		}
		value := parsed.UTC()
		return &value
	}
	return nil
}

func normalizeTavilyURL(
	raw string,
) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil ||
		parsed.Scheme == "" ||
		parsed.Host == "" {
		return ""
	}
	parsed.Fragment = ""
	return parsed.String()
}

func tavilyPublisher(
	rawURL string,
) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(
		strings.TrimSpace(parsed.Hostname()),
	)
}

func normalizeTavilyText(
	value string,
) string {
	return strings.Join(
		strings.Fields(
			strings.TrimSpace(value),
		),
		" ",
	)
}

func tavilyCoverageError(
	total int,
	usable int,
	missingBody int,
	missingDate int,
) error {
	if total == 0 {
		return nil
	}
	if missingBody == 0 &&
		missingDate == 0 {
		return nil
	}

	return fmt.Errorf(
		"tavily web search produced %d usable full-content result(s) from %d hit(s): %d missing raw_content, %d missing/invalid published_date",
		usable,
		total,
		missingBody,
		missingDate,
	)
}
