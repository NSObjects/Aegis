package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	cninfoStockIndexURL   = "https://www.cninfo.com.cn/new/data/szse_stock.json"
	cninfoAnnouncementURL = "https://www.cninfo.com.cn/new/hisAnnouncement/query"
	cninfoStaticBaseURL   = "https://static.cninfo.com.cn/"
	cninfoPublisher       = "巨潮资讯网"

	defaultCninfoPageSize       = 30
	defaultCninfoMaxPages       = 3
	defaultCninfoMaxPDFBytes    = int64(20 << 20)
	defaultCninfoMaxTextChars   = 120_000
	defaultCninfoBodyErrorChars = 256
)

var (
	cninfoShanghaiLocation = time.FixedZone("Asia/Shanghai", 8*60*60)
	cninfoHTMLTagPattern    = regexp.MustCompile("<[^>]+>")
)

// CninfoAnnouncementRetriever retrieves official A-share announcement metadata
// from Cninfo. It intentionally implements only one concrete acquisition path;
// multi-source planning belongs above TaskRunner.
type CninfoAnnouncementRetriever struct {
	Client       HTTPDoer
	Now          func() time.Time
	PageSize     int
	MaxPages     int
	PDFExtractor PDFTextExtractor
	MaxPDFBytes  int64
	MaxTextChars int
}

func (CninfoAnnouncementRetriever) ID() string {
	return "cninfo_announcements"
}

func (CninfoAnnouncementRetriever) Method() AcquisitionMethod {
	return AcquisitionAPI
}

func (r CninfoAnnouncementRetriever) Retrieve(
	ctx context.Context,
	task ResearchTask,
) ([]RawDocument, error) {
	if task.InformationNeed != "" &&
		task.InformationNeed != NeedOfficialDisclosure {
		return nil, fmt.Errorf(
			"cninfo announcement retriever does not support information need %q",
			task.InformationNeed,
		)
	}

	symbol, err := normalizeCninfoSymbol(task.Symbol)
	if err != nil {
		return nil, err
	}

	start, end, err := cninfoEffectiveQueryRange(task)
	if err != nil {
		return nil, err
	}

	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	stock, err := r.resolveStock(ctx, client, symbol)
	if err != nil {
		return nil, err
	}

	pageSize := r.PageSize
	if pageSize <= 0 {
		pageSize = defaultCninfoPageSize
	}
	if pageSize > 100 {
		pageSize = 100
	}

	maxPages := r.MaxPages
	if maxPages <= 0 {
		maxPages = defaultCninfoMaxPages
	}

	now := r.Now
	if now == nil {
		now = time.Now
	}
	retrievedAt := now().UTC()

	documents := make([]RawDocument, 0)
	var retrievalErr error

	for page := 1; page <= maxPages; page++ {
		payload, err := r.fetchAnnouncementPage(
			ctx,
			client,
			stock,
			start,
			end,
			page,
			pageSize,
		)
		if err != nil {
			retrievalErr = fmt.Errorf(
				"cninfo announcements page %d: %w",
				page,
				err,
			)
			break
		}

		for _, item := range payload.Announcements {
			document := cninfoRawDocument(
				task,
				stock,
				item,
				retrievedAt,
			)
			if document == nil {
				continue
			}
			if !withinResearchTimeRange(
				document.PublishedAt,
				task.TimeRange,
			) {
				continue
			}
			documents = append(documents, *document)
		}

		if payload.TotalPages > 0 && page >= payload.TotalPages {
			break
		}
		if len(payload.Announcements) < pageSize {
			break
		}
	}

	if len(documents) == 0 {
		return nil, retrievalErr
	}

	enrichmentErr := r.enrichPDFBodies(
		ctx,
		client,
		task,
		documents,
	)

	return documents, errors.Join(
		retrievalErr,
		enrichmentErr,
	)
}

type cninfoStock struct {
	Symbol       string
	InstrumentID string
	Exchange     string
	OrgID        string
	Name         string
}

type cninfoAnnouncementPage struct {
	Announcements []map[string]any `json:"announcements"`
	TotalPages    int              `json:"totalpages"`
}

func (r CninfoAnnouncementRetriever) resolveStock(
	ctx context.Context,
	client HTTPDoer,
	symbol string,
) (cninfoStock, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		cninfoStockIndexURL,
		nil,
	)
	if err != nil {
		return cninfoStock{}, fmt.Errorf(
			"build cninfo stock index request: %w",
			err,
		)
	}
	setCninfoHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return cninfoStock{}, fmt.Errorf(
			"request cninfo stock index: %w",
			err,
		)
	}
	defer resp.Body.Close()

	if err := requireHTTP2xx(resp, "cninfo stock index"); err != nil {
		return cninfoStock{}, err
	}

	var payload struct {
		StockList []struct {
			Code  string `json:"code"`
			OrgID string `json:"orgId"`
			Name  string `json:"zwjc"`
		} `json:"stockList"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return cninfoStock{}, fmt.Errorf(
			"decode cninfo stock index: %w",
			err,
		)
	}

	for _, item := range payload.StockList {
		if strings.TrimSpace(item.Code) != symbol {
			continue
		}
		orgID := strings.TrimSpace(item.OrgID)
		if orgID == "" {
			return cninfoStock{}, fmt.Errorf(
				"cninfo stock %s is missing orgId",
				symbol,
			)
		}

		exchange := cninfoExchange(symbol)
		return cninfoStock{
			Symbol:       symbol,
			InstrumentID: symbol + "." + cninfoExchangeSuffix(exchange),
			Exchange:     exchange,
			OrgID:        orgID,
			Name:         cleanCninfoText(item.Name),
		}, nil
	}

	return cninfoStock{}, fmt.Errorf(
		"cninfo stock index does not contain symbol %s",
		symbol,
	)
}

func (r CninfoAnnouncementRetriever) fetchAnnouncementPage(
	ctx context.Context,
	client HTTPDoer,
	stock cninfoStock,
	start time.Time,
	end time.Time,
	page int,
	pageSize int,
) (cninfoAnnouncementPage, error) {
	form := url.Values{
		"pageNum":   {strconv.Itoa(page)},
		"pageSize":  {strconv.Itoa(pageSize)},
		"column":    {cninfoColumn(stock.Symbol)},
		"tabName":   {"fulltext"},
		"plate":     {cninfoPlate(stock.Symbol)},
		"stock":     {stock.Symbol + "," + stock.OrgID},
		"searchkey": {""},
		"secid":     {""},
		"category":  {""},
		"trade":     {""},
		"seDate":    {cninfoDateRange(start, end)},
		"sortName":  {""},
		"sortType":  {""},
		"isHLtitle": {"true"},
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		cninfoAnnouncementURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return cninfoAnnouncementPage{}, fmt.Errorf(
			"build cninfo announcement request: %w",
			err,
		)
	}
	setCninfoHeaders(req)
	req.Header.Set(
		"Content-Type",
		"application/x-www-form-urlencoded; charset=UTF-8",
	)
	req.Header.Set("Origin", "https://www.cninfo.com.cn")

	resp, err := client.Do(req)
	if err != nil {
		return cninfoAnnouncementPage{}, fmt.Errorf(
			"request cninfo announcements: %w",
			err,
		)
	}
	defer resp.Body.Close()

	if err := requireHTTP2xx(resp, "cninfo announcements"); err != nil {
		return cninfoAnnouncementPage{}, err
	}

	var payload cninfoAnnouncementPage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return cninfoAnnouncementPage{}, fmt.Errorf(
			"decode cninfo announcements: %w",
			err,
		)
	}
	return payload, nil
}

func cninfoRawDocument(
	task ResearchTask,
	stock cninfoStock,
	item map[string]any,
	retrievedAt time.Time,
) *RawDocument {
	announcementID := cleanCninfoText(
		fmt.Sprint(item["announcementId"]),
	)
	title := cleanCninfoText(
		firstCninfoValue(
			item["announcementTitle"],
			item["shortTitle"],
		),
	)
	adjunctURL := cleanCninfoText(
		fmt.Sprint(item["adjunctUrl"]),
	)

	if announcementID == "<nil>" {
		announcementID = ""
	}
	if adjunctURL == "<nil>" {
		adjunctURL = ""
	}

	publishedAt := cninfoAnnouncementTime(
		item["announcementTime"],
	)

	document := &RawDocument{
		TaskID:      task.ID,
		Title:       title,
		URL:         cninfoDownloadURL(adjunctURL),
		Publisher:    cninfoPublisher,
		SourceType:   SourceOfficial,
		Authority:    AuthorityIssuer,
		EvidenceKind: EvidenceDisclosure,
		IsPrimary:    true,
		PublishedAt: publishedAt,
		FirstSeenAt: cloneTime(&retrievedAt),
		AvailableAt: cloneTime(publishedAt),
		RetrievedAt: retrievedAt,
		RawMetadata: map[string]any{
			"provider":        "cninfo",
			"announcement_id": announcementID,
			"instrument_id":   stock.InstrumentID,
			"symbol":          stock.Symbol,
			"exchange":        stock.Exchange,
			"company": cleanCninfoText(
				firstCninfoValue(
					item["secName"],
					stock.Name,
				),
			),
			"file_type": firstCninfoValue(
				item["adjunctType"],
			),
			"file_size_kb": item["adjunctSize"],
			"source_url":   cninfoAnnouncementURL,
		},
	}
	if announcementID != "" {
		document.ID = "cninfo_announcement_" + announcementID
	}

	if document.Title == "" &&
		document.URL == "" &&
		document.PublishedAt == nil {
		return nil
	}
	return document
}

func (r CninfoAnnouncementRetriever) enrichPDFBodies(
	ctx context.Context,
	client HTTPDoer,
	task ResearchTask,
	documents []RawDocument,
) error {
	maxPDFBytes := r.MaxPDFBytes
	if maxPDFBytes <= 0 {
		maxPDFBytes = defaultCninfoMaxPDFBytes
	}

	maxTextChars := r.MaxTextChars
	if maxTextChars <= 0 {
		maxTextChars = defaultCninfoMaxTextChars
	}

	extractor := r.PDFExtractor
	if extractor == nil {
		extractor = PDFiumTextExtractor{
			MaxTextChars: maxTextChars,
		}
	}

	failed := 0
	samples := make([]string, 0, 3)

	for index := range documents {
		if err := ctx.Err(); err != nil {
			return errors.Join(
				cninfoPDFEnrichmentError(
					failed,
					samples,
				),
				err,
			)
		}

		document := &documents[index]

		// The provider can return the rest of the current day even when
		// AsOf is earlier. Canonicalization still owns the PIT guard, but
		// there is no reason to download a body we already know is beyond
		// the request cutoff.
		if !task.AsOf.IsZero() &&
			document.PublishedAt != nil &&
			document.PublishedAt.After(task.AsOf) {
			continue
		}

		if strings.TrimSpace(document.URL) == "" {
			err := errors.New(
				"announcement PDF URL is missing",
			)
			recordCninfoBodyFailure(
				document,
				0,
				err,
			)
			failed++
			samples = appendCninfoBodyErrorSample(
				samples,
				document.ID,
				err,
			)
			continue
		}

		data, err := downloadCninfoPDF(
			ctx,
			client,
			document.URL,
			maxPDFBytes,
		)
		if err != nil {
			recordCninfoBodyFailure(
				document,
				0,
				err,
			)
			failed++
			samples = appendCninfoBodyErrorSample(
				samples,
				document.ID,
				err,
			)
			if ctx.Err() != nil {
				return errors.Join(
					cninfoPDFEnrichmentError(
						failed,
						samples,
					),
					ctx.Err(),
				)
			}
			continue
		}

		extraction, err := extractor.Extract(
			ctx,
			data,
		)
		if err != nil {
			recordCninfoBodyFailure(
				document,
				len(data),
				err,
			)
			failed++
			samples = appendCninfoBodyErrorSample(
				samples,
				document.ID,
				err,
			)
			if ctx.Err() != nil {
				return errors.Join(
					cninfoPDFEnrichmentError(
						failed,
						samples,
					),
					ctx.Err(),
				)
			}
			continue
		}

		text := normalizePDFText(extraction.Text)
		text, bounded := truncateTextChars(
			text,
			maxTextChars,
		)
		truncated := extraction.Truncated || bounded
		if strings.TrimSpace(text) == "" {
			err := errors.New(
				"PDF contains no extractable text",
			)
			recordCninfoBodyFailure(
				document,
				len(data),
				err,
			)
			failed++
			samples = appendCninfoBodyErrorSample(
				samples,
				document.ID,
				err,
			)
			continue
		}

		document.Content = text
		recordCninfoBodySuccess(
			document,
			len(data),
			utf8.RuneCountInString(text),
			truncated,
		)
	}

	return cninfoPDFEnrichmentError(
		failed,
		samples,
	)
}

func downloadCninfoPDF(
	ctx context.Context,
	client HTTPDoer,
	rawURL string,
	maxBytes int64,
) ([]byte, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		rawURL,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"build cninfo PDF request: %w",
			err,
		)
	}
	setCninfoHeaders(req)
	req.Header.Set(
		"Accept",
		"application/pdf,application/octet-stream;q=0.9,*/*;q=0.1",
	)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"request cninfo PDF: %w",
			err,
		)
	}
	defer resp.Body.Close()

	if err := requireHTTP2xx(
		resp,
		"cninfo announcement PDF",
	); err != nil {
		return nil, err
	}

	if maxBytes <= 0 {
		maxBytes = defaultCninfoMaxPDFBytes
	}
	if resp.ContentLength > maxBytes {
		return nil, fmt.Errorf(
			"cninfo PDF body exceeds %d-byte limit",
			maxBytes,
		)
	}

	data, err := io.ReadAll(
		io.LimitReader(
			resp.Body,
			maxBytes+1,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"read cninfo PDF body: %w",
			err,
		)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf(
			"cninfo PDF body exceeds %d-byte limit",
			maxBytes,
		)
	}
	if len(data) == 0 {
		return nil, errors.New(
			"cninfo PDF body is empty",
		)
	}
	return data, nil
}

func recordCninfoBodySuccess(
	document *RawDocument,
	bodyBytes int,
	textChars int,
	truncated bool,
) {
	if document == nil {
		return
	}
	if document.RawMetadata == nil {
		document.RawMetadata = make(map[string]any)
	}
	document.RawMetadata["body_format"] = "pdf"
	document.RawMetadata["body_enriched"] = true
	document.RawMetadata["body_bytes"] = bodyBytes
	document.RawMetadata["text_chars"] = textChars
	document.RawMetadata["body_truncated"] = truncated
	delete(document.RawMetadata, "body_error")
}

func recordCninfoBodyFailure(
	document *RawDocument,
	bodyBytes int,
	err error,
) {
	if document == nil {
		return
	}
	if document.RawMetadata == nil {
		document.RawMetadata = make(map[string]any)
	}
	document.RawMetadata["body_format"] = "pdf"
	document.RawMetadata["body_enriched"] = false
	if bodyBytes > 0 {
		document.RawMetadata["body_bytes"] = bodyBytes
	}
	document.RawMetadata["body_error"] =
		boundedCninfoBodyError(err)
}

func boundedCninfoBodyError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.Join(
		strings.Fields(err.Error()),
		" ",
	)
	value, _ = truncateTextChars(
		value,
		defaultCninfoBodyErrorChars,
	)
	return value
}

func appendCninfoBodyErrorSample(
	samples []string,
	documentID string,
	err error,
) []string {
	if len(samples) >= 3 {
		return samples
	}
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		documentID = "unknown"
	}
	return append(
		samples,
		documentID+": "+boundedCninfoBodyError(err),
	)
}

func cninfoPDFEnrichmentError(
	failed int,
	samples []string,
) error {
	if failed <= 0 {
		return nil
	}

	message := fmt.Sprintf(
		"cninfo PDF body enrichment failed for %d announcement(s)",
		failed,
	)
	if len(samples) > 0 {
		message += ": " + strings.Join(
			samples,
			"; ",
		)
	}
	return errors.New(message)
}

func normalizeCninfoSymbol(raw string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	switch {
	case strings.HasSuffix(value, ".SH"),
		strings.HasSuffix(value, ".SZ"),
		strings.HasSuffix(value, ".BJ"):
		value = value[:len(value)-3]
	case len(value) >= 8 &&
		(value[:2] == "SH" ||
			value[:2] == "SZ" ||
			value[:2] == "BJ"):
		value = value[2:]
	}

	if len(value) != 6 {
		return "", fmt.Errorf(
			"cninfo symbol must be a six-digit A-share code: %q",
			raw,
		)
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return "", fmt.Errorf(
				"cninfo symbol must be a six-digit A-share code: %q",
				raw,
			)
		}
	}
	return value, nil
}

func cninfoEffectiveQueryRange(
	task ResearchTask,
) (time.Time, time.Time, error) {
	start := task.TimeRange.Start
	end := task.TimeRange.End

	if !task.AsOf.IsZero() &&
		(end.IsZero() || task.AsOf.Before(end)) {
		end = task.AsOf
	}
	if !start.IsZero() &&
		!end.IsZero() &&
		start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf(
			"event research time range start %s is after effective end %s",
			start.Format(time.RFC3339),
			end.Format(time.RFC3339),
		)
	}

	return start, end, nil
}

func cninfoDateRange(
	start time.Time,
	end time.Time,
) string {
	if start.IsZero() && end.IsZero() {
		return ""
	}

	startText := ""
	if !start.IsZero() {
		startText = start.In(cninfoShanghaiLocation).
			Format("2006-01-02")
	}
	endText := ""
	if !end.IsZero() {
		endText = end.In(cninfoShanghaiLocation).
			Format("2006-01-02")
	}

	return startText + "~" + endText
}

func cninfoAnnouncementTime(value any) *time.Time {
	var millis int64
	switch typed := value.(type) {
	case float64:
		millis = int64(typed)
	case float32:
		millis = int64(typed)
	case int64:
		millis = typed
	case int:
		millis = int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return nil
		}
		millis = parsed
	case string:
		parsed, err := strconv.ParseInt(
			strings.TrimSpace(typed),
			10,
			64,
		)
		if err != nil {
			return nil
		}
		millis = parsed
	default:
		return nil
	}

	if millis <= 0 {
		return nil
	}
	valueTime := time.UnixMilli(millis).
		In(cninfoShanghaiLocation)
	return &valueTime
}

func withinResearchTimeRange(
	value *time.Time,
	timeRange TimeRange,
) bool {
	if value == nil {
		return true
	}
	if !timeRange.Start.IsZero() &&
		value.Before(timeRange.Start) {
		return false
	}
	if !timeRange.End.IsZero() &&
		value.After(timeRange.End) {
		return false
	}
	return true
}

func cninfoDownloadURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "https://") {
		return path
	}
	if strings.HasPrefix(path, "http://static.cninfo.com.cn/") {
		return cninfoStaticBaseURL +
			strings.TrimPrefix(
				path,
				"http://static.cninfo.com.cn/",
			)
	}
	if strings.HasPrefix(path, "http://") {
		return path
	}
	return cninfoStaticBaseURL +
		strings.TrimLeft(path, "/")
}

func cleanCninfoText(value string) string {
	value = html.UnescapeString(value)
	value = cninfoHTMLTagPattern.ReplaceAllString(
		value,
		"",
	)
	return strings.Join(strings.Fields(value), " ")
}

func firstCninfoValue(values ...any) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		text := cleanCninfoText(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func cninfoExchange(symbol string) string {
	switch {
	case strings.HasPrefix(symbol, "92"),
		strings.HasPrefix(symbol, "4"),
		strings.HasPrefix(symbol, "8"):
		return "BSE"
	case strings.HasPrefix(symbol, "6"),
		strings.HasPrefix(symbol, "9"),
		strings.HasPrefix(symbol, "5"):
		return "SSE"
	default:
		return "SZSE"
	}
}

func cninfoExchangeSuffix(exchange string) string {
	switch exchange {
	case "SSE":
		return "SH"
	case "BSE":
		return "BJ"
	default:
		return "SZ"
	}
}

func cninfoColumn(symbol string) string {
	switch cninfoExchange(symbol) {
	case "SSE":
		return "sse"
	case "BSE":
		return "bj"
	default:
		return "szse"
	}
}

func cninfoPlate(symbol string) string {
	switch cninfoExchange(symbol) {
	case "SSE":
		return "sh"
	case "BSE":
		return "bj"
	default:
		return "sz"
	}
}

func setCninfoHeaders(req *http.Request) {
	req.Header.Set(
		"User-Agent",
		"Mozilla/5.0 Aegis/1.0",
	)
	req.Header.Set(
		"Accept",
		"application/json,text/plain,*/*",
	)
	req.Header.Set(
		"Referer",
		"https://www.cninfo.com.cn/",
	)
}
