package event

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	clsTelegraphURL = "https://api3.cls.cn/v1/roll/get_roll_list"
	clsPublisher    = "财联社"

	defaultCLSNewsLimit   = 20
	defaultCLSNewsMaxDays = 7
)

var clsShanghaiLocation = time.FixedZone(
	"Asia/Shanghai",
	8*60*60,
)

type CLSNewsCategory string

const (
	CLSNewsAll       CLSNewsCategory = "all"
	CLSNewsImportant CLSNewsCategory = "important"
	CLSNewsCompany   CLSNewsCategory = "company"
)

type ClsNewsRetriever struct {
	Client   HTTPDoer
	Now      func() time.Time
	Category CLSNewsCategory
	Limit    int
	MaxDays  int
}

func (ClsNewsRetriever) ID() string {
	return "cls_news_telegraph"
}

func (ClsNewsRetriever) Method() AcquisitionMethod {
	return AcquisitionAPI
}

func (r ClsNewsRetriever) Retrieve(
	ctx context.Context,
	task ResearchTask,
) ([]RawDocument, error) {
	if task.InformationNeed != "" &&
		task.InformationNeed != NeedBreakingNews {
		return nil, fmt.Errorf(
			"CLS news retriever does not support information need %q",
			task.InformationNeed,
		)
	}

	category, err := normalizeCLSNewsCategory(r.Category)
	if err != nil {
		return nil, err
	}

	now := r.Now
	if now == nil {
		now = time.Now
	}
	retrievedAt := now().UTC()

	start, end, err := clsEffectiveQueryRange(
		task,
		retrievedAt,
	)
	if err != nil {
		return nil, err
	}

	limit := r.Limit
	if limit <= 0 {
		limit = defaultCLSNewsLimit
	}
	if limit > 100 {
		limit = 100
	}

	maxDays := r.MaxDays
	if maxDays <= 0 {
		maxDays = defaultCLSNewsMaxDays
	}

	days := clsQueryDays(start, end)
	if len(days) > maxDays {
		return nil, fmt.Errorf(
			"CLS news query spans %d days; max %d",
			len(days),
			maxDays,
		)
	}

	client := r.Client
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	documents := make([]RawDocument, 0)
	for _, day := range days {
		lastTime := clsEndOfDay(day)
		if end.Before(lastTime) {
			lastTime = end
		}

		items, err := r.fetchDay(
			ctx,
			client,
			category,
			lastTime,
			limit,
		)
		if err != nil {
			if len(documents) > 0 {
				return documents, fmt.Errorf(
					"CLS news %s: %w",
					day.Format("2006-01-02"),
					err,
				)
			}
			return nil, fmt.Errorf(
				"CLS news %s: %w",
				day.Format("2006-01-02"),
				err,
			)
		}

		dayStart := clsStartOfDay(day)
		dayEnd := clsEndOfDay(day)
		for _, item := range items {
			document := clsRawDocument(
				category,
				item,
				retrievedAt,
			)
			if document == nil ||
				document.PublishedAt == nil {
				continue
			}
			if document.PublishedAt.Before(dayStart) ||
				document.PublishedAt.After(dayEnd) {
				continue
			}
			if !withinResearchTimeRange(
				document.PublishedAt,
				task.TimeRange,
			) {
				continue
			}
			documents = append(
				documents,
				*document,
			)
		}
	}

	return documents, nil
}

type clsNewsItem struct {
	ID       any    `json:"id"`
	NewsID   any    `json:"news_id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Ctime    any    `json:"ctime"`
	ShareURL string `json:"shareurl"`
}

func (r ClsNewsRetriever) fetchDay(
	ctx context.Context,
	client HTTPDoer,
	category CLSNewsCategory,
	lastTime time.Time,
	limit int,
) ([]clsNewsItem, error) {
	params := clsSignedParams(map[string]string{
		"refresh_type": "1",
		"last_time": strconv.FormatInt(
			lastTime.Unix(),
			10,
		),
		"rn": strconv.Itoa(limit),
	})

	if apiCategory := clsCategoryParam(category);
		apiCategory != "" {
		delete(params, "sign")
		params["category"] = apiCategory
		params["sign"] = clsSign(params)
	}

	query := url.Values{}
	for key, value := range params {
		query.Set(key, value)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		clsTelegraphURL+"?"+query.Encode(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"build CLS news request: %w",
			err,
		)
	}

	req.Header.Set(
		"User-Agent",
		"okhttp/4.9.0",
	)
	req.Header.Set(
		"Accept",
		"application/json,text/plain,*/*",
	)
	req.Header.Set(
		"Referer",
		"https://www.cls.cn/",
	)
	req.Host = "api3.cls.cn"

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"request CLS news: %w",
			err,
		)
	}
	defer resp.Body.Close()

	if err := requireHTTP2xx(
		resp,
		"CLS news telegraph",
	); err != nil {
		return nil, err
	}

	var payload struct {
		Errno any `json:"errno"`
		Data struct {
			RollData []clsNewsItem `json:"roll_data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(
		&payload,
	); err != nil {
		return nil, fmt.Errorf(
			"decode CLS news: %w",
			err,
		)
	}
	if !clsErrnoOK(payload.Errno) {
		return nil, fmt.Errorf(
			"CLS news returned errno %v",
			payload.Errno,
		)
	}

	return payload.Data.RollData, nil
}

func clsRawDocument(
	category CLSNewsCategory,
	item clsNewsItem,
	retrievedAt time.Time,
) *RawDocument {
	newsID := firstCLSValue(
		item.ID,
		item.NewsID,
	)
	publishedAt := clsNewsTime(item.Ctime)
	title := cleanCLSText(item.Title)
	content := cleanCLSText(item.Content)

	document := &RawDocument{
		Title:       title,
		Content:     content,
		Publisher:   clsPublisher,
		SourceType:  SourceIndustryMedia,
		IsPrimary:   false,
		PublishedAt: publishedAt,
		RetrievedAt: retrievedAt,
		RawMetadata: map[string]any{
			"provider":   "cls",
			"news_id":    newsID,
			"category":   string(category),
			"ctime":      item.Ctime,
			"share_url":  strings.TrimSpace(item.ShareURL),
			"source_url": clsTelegraphURL,
		},
	}

	if newsID != "" {
		document.ID = "cls_news_" + newsID
		document.URL = "https://www.cls.cn/detail/" +
			newsID
	}

	if document.Title == "" &&
		document.Content == "" &&
		document.URL == "" {
		return nil
	}
	return document
}

func normalizeCLSNewsCategory(
	category CLSNewsCategory,
) (CLSNewsCategory, error) {
	if category == "" {
		return CLSNewsImportant, nil
	}
	switch category {
	case CLSNewsAll,
		CLSNewsImportant,
		CLSNewsCompany:
		return category, nil
	default:
		return "", fmt.Errorf(
			"CLS news category must be all, important, or company: %q",
			category,
		)
	}
}

func clsCategoryParam(
	category CLSNewsCategory,
) string {
	switch category {
	case CLSNewsImportant:
		return "red"
	case CLSNewsCompany:
		return "announcement"
	default:
		return ""
	}
}

func clsEffectiveQueryRange(
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
		start = clsStartOfDay(end)
	}

	if start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf(
			"event research time range start %s is after effective end %s",
			start.Format(time.RFC3339),
			end.Format(time.RFC3339),
		)
	}
	return start, end, nil
}

func clsQueryDays(
	start time.Time,
	end time.Time,
) []time.Time {
	first := clsStartOfDay(start)
	last := clsStartOfDay(end)

	days := make([]time.Time, 0)
	for day := first;
		!day.After(last);
		day = day.AddDate(0, 0, 1) {
		days = append(days, day)
	}
	return days
}

func clsStartOfDay(value time.Time) time.Time {
	local := value.In(clsShanghaiLocation)
	return time.Date(
		local.Year(),
		local.Month(),
		local.Day(),
		0, 0, 0, 0,
		clsShanghaiLocation,
	)
}

func clsEndOfDay(value time.Time) time.Time {
	return clsStartOfDay(value).
		AddDate(0, 0, 1).
		Add(-time.Second)
}

func clsNewsTime(value any) *time.Time {
	var seconds int64
	switch typed := value.(type) {
	case float64:
		seconds = int64(typed)
	case float32:
		seconds = int64(typed)
	case int64:
		seconds = typed
	case int:
		seconds = int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return nil
		}
		seconds = parsed
	case string:
		parsed, err := strconv.ParseInt(
			strings.TrimSpace(typed),
			10,
			64,
		)
		if err != nil {
			return nil
		}
		seconds = parsed
	default:
		return nil
	}

	if seconds <= 0 {
		return nil
	}
	publishedAt := time.Unix(
		seconds,
		0,
	).In(clsShanghaiLocation)
	return &publishedAt
}

func clsSignedParams(
	extra map[string]string,
) map[string]string {
	params := map[string]string{
		"app":           "cailianpress",
		"sv":            "8.7.4",
		"os":            "android",
		"mb":            "Xiaomi-2206123SC",
		"ov":            "32",
		"channel":       "8",
		"motif":         "0",
		"net":           "",
		"province_code": "3205",
		"token":         "",
		"uid":           "",
	}
	for key, value := range extra {
		params[key] = value
	}
	params["sign"] = clsSign(params)
	return params
}

func clsSign(
	params map[string]string,
) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if key == "sign" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(
			parts,
			key+"="+params[key],
		)
	}
	raw := strings.Join(parts, "&")

	sha1Sum := sha1.Sum([]byte(raw))
	sha1Hex := hex.EncodeToString(sha1Sum[:])
	md5Sum := md5.Sum([]byte(sha1Hex))
	return hex.EncodeToString(md5Sum[:])
}

func clsErrnoOK(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case float64:
		return typed == 0
	case float32:
		return typed == 0
	case int:
		return typed == 0
	case int64:
		return typed == 0
	case json.Number:
		return typed.String() == "0"
	case string:
		return strings.TrimSpace(typed) == "0"
	default:
		return false
	}
}

func firstCLSValue(values ...any) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		text := strings.TrimSpace(
			fmt.Sprint(value),
		)
		if text != "" &&
			text != "<nil>" {
			return text
		}
	}
	return ""
}

func cleanCLSText(value string) string {
	return strings.Join(
		strings.Fields(
			strings.TrimSpace(value),
		),
		" ",
	)
}
