package event

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCLSSignMatchesAxDataReference(t *testing.T) {
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
		"refresh_type":  "1",
		"last_time":     "1790261999",
		"rn":            "20",
	}

	const want = "ab8b21c2fe16e400bdc53bc0b0f692cf"
	if got := clsSign(params); got != want {
		t.Fatalf(
			"clsSign() = %q, want %q",
			got,
			want,
		)
	}
}

func TestCLSNewsRetrieverRunsThroughTaskRunner(t *testing.T) {
	location := clsShanghaiLocation
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, location,
	)
	before := time.Date(
		2026, 9, 29, 10, 0, 0, 0, location,
	)
	after := time.Date(
		2026, 9, 29, 16, 0, 0, 0, location,
	)
	retrievedAt := asOf.Add(-time.Minute)

	client := httpDoerFunc(
		func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/v1/roll/get_roll_list" {
				t.Fatalf(
					"unexpected path = %q",
					req.URL.Path,
				)
			}
			query := req.URL.Query()
			if got := query.Get("category"); got != "red" {
				t.Fatalf(
					"category = %q, want red",
					got,
				)
			}
			if got := query.Get("last_time");
				got != strconv.FormatInt(asOf.Unix(), 10) {
				t.Fatalf(
					"last_time = %q, want %d",
					got,
					asOf.Unix(),
				)
			}
			if query.Get("sign") == "" {
				t.Fatal("CLS request must include sign")
			}

			signParams := make(map[string]string)
			for key, values := range query {
				if key == "sign" || len(values) == 0 {
					continue
				}
				signParams[key] = values[0]
			}
			if got, want := query.Get("sign"), clsSign(signParams);
				got != want {
				t.Fatalf(
					"sign = %q, want %q",
					got,
					want,
				)
			}

			return jsonHTTPResponse(
				http.StatusOK,
				map[string]any{
					"errno": 0,
					"data": map[string]any{
						"roll_data": []map[string]any{
							{
								"id":      "2450001",
								"title":   "盘中重要快讯",
								"content": "第一条新闻正文",
								"ctime":   before.Unix(),
							},
							{
								"id":      "2450002",
								"title":   "未来快讯",
								"content": "as_of 之后的信息",
								"ctime":   after.Unix(),
							},
						},
					},
				},
			), nil
		},
	)

	task := ResearchTask{
		ID:              "task-cls-news",
		InformationNeed: NeedBreakingNews,
		AsOf:            asOf,
		TimeRange: TimeRange{
			Start: time.Date(
				2026, 9, 29, 0, 0, 0, 0, location,
			),
			End: time.Date(
				2026, 9, 29, 18, 0, 0, 0, location,
			),
		},
		Requirements: ResearchRequirements{
			PITSafe:             true,
			RequiredSourceTypes: []SourceType{SourceIndustryMedia},
			RequiredAuthorities: []EvidenceAuthority{
				AuthorityMedia,
			},
			RequiredEvidenceKinds: []EvidenceKind{
				EvidenceNewsReport,
			},
			MinDocuments:          1,
			MinIndependentSources: 1,
		},
	}

	output, err := (TaskRunner{
		Now: func() time.Time {
			return retrievedAt
		},
	}).Run(
		context.Background(),
		task,
		ClsNewsRetriever{
			Client:   client,
			Category: CLSNewsImportant,
			Now: func() time.Time {
				return retrievedAt
			},
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if output.Attempt.Status != AcquisitionSucceeded {
		t.Fatalf(
			"attempt status = %q, want %q",
			output.Attempt.Status,
			AcquisitionSucceeded,
		)
	}
	if len(output.RawDocuments) != 2 {
		t.Fatalf(
			"raw documents = %d, want 2",
			len(output.RawDocuments),
		)
	}
	if len(output.Canonicalization.Documents) != 1 {
		t.Fatalf(
			"canonical documents = %d, want 1",
			len(output.Canonicalization.Documents),
		)
	}
	if len(output.Canonicalization.Rejected) != 1 {
		t.Fatalf(
			"rejected documents = %d, want 1",
			len(output.Canonicalization.Rejected),
		)
	}
	if got := output.Canonicalization.Rejected[0].Reason;
		got != RejectionFuturePublishedAt {
		t.Fatalf(
			"rejection reason = %q, want %q",
			got,
			RejectionFuturePublishedAt,
		)
	}
	if output.Result.Status != TaskSatisfied {
		t.Fatalf(
			"task status = %q, want %q; gaps=%v",
			output.Result.Status,
			TaskSatisfied,
			output.Result.Gaps,
		)
	}

	raw := output.RawDocuments[0]
	if raw.ID != "cls_news_2450001" {
		t.Fatalf("raw id = %q", raw.ID)
	}
	if raw.URL != "https://www.cls.cn/detail/2450001" {
		t.Fatalf("raw URL = %q", raw.URL)
	}
	if raw.Publisher != clsPublisher {
		t.Fatalf("publisher = %q", raw.Publisher)
	}
	if raw.SourceType != SourceIndustryMedia {
		t.Fatalf("source type = %q", raw.SourceType)
	}
	if raw.Authority != AuthorityMedia {
		t.Fatalf("authority = %q", raw.Authority)
	}
	if raw.EvidenceKind != EvidenceNewsReport {
		t.Fatalf("evidence kind = %q", raw.EvidenceKind)
	}
	if raw.IsPrimary {
		t.Fatal("CLS media evidence must not be primary")
	}
	if raw.PublishedAt == nil ||
		!raw.PublishedAt.Equal(before) {
		t.Fatalf(
			"published_at = %v, want %s",
			raw.PublishedAt,
			before,
		)
	}
	if raw.AvailableAt == nil ||
		!raw.AvailableAt.Equal(retrievedAt) {
		t.Fatalf(
			"available_at = %v, want retrieval time %s",
			raw.AvailableAt,
			retrievedAt,
		)
	}
	if raw.FirstSeenAt == nil ||
		!raw.FirstSeenAt.Equal(retrievedAt) {
		t.Fatalf(
			"first_seen_at = %v, want retrieval time %s",
			raw.FirstSeenAt,
			retrievedAt,
		)
	}
	if raw.RawMetadata["category"] != "important" {
		t.Fatalf(
			"category metadata = %v",
			raw.RawMetadata["category"],
		)
	}
}


func TestCLSHistoricalBodyRequiresHistoricalSnapshot(
	t *testing.T,
) {
	location := clsShanghaiLocation
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, location,
	)
	publishedAt := asOf.Add(-4 * time.Hour)
	retrievedAt := asOf.Add(time.Hour)

	client := httpDoerFunc(
		func(*http.Request) (*http.Response, error) {
			return jsonHTTPResponse(
				http.StatusOK,
				map[string]any{
					"errno": 0,
					"data": map[string]any{
						"roll_data": []map[string]any{
							{
								"id":      "2450099",
								"title":   "历史快讯",
								"content": "今天重新抓取到的当前正文",
								"ctime":   publishedAt.Unix(),
							},
						},
					},
				},
			), nil
		},
	)

	output, err := (TaskRunner{
		Now: func() time.Time {
			return retrievedAt
		},
	}).Run(
		context.Background(),
		ResearchTask{
			ID:              "task-cls-historical-snapshot",
			InformationNeed: NeedBreakingNews,
			AsOf:            asOf,
			TimeRange: TimeRange{
				Start: clsStartOfDay(asOf),
				End:   asOf,
			},
			Requirements: ResearchRequirements{
				PITSafe:      true,
				MinDocuments: 1,
			},
		},
		ClsNewsRetriever{
			Client: client,
			Now: func() time.Time {
				return retrievedAt
			},
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(output.RawDocuments) != 1 {
		t.Fatalf(
			"raw documents = %d, want 1",
			len(output.RawDocuments),
		)
	}
	raw := output.RawDocuments[0]
	if raw.PublishedAt == nil ||
		!raw.PublishedAt.Equal(publishedAt) {
		t.Fatalf(
			"published_at = %v, want %s",
			raw.PublishedAt,
			publishedAt,
		)
	}
	if raw.AvailableAt == nil ||
		!raw.AvailableAt.Equal(retrievedAt) {
		t.Fatalf(
			"available_at = %v, want retrieval time %s",
			raw.AvailableAt,
			retrievedAt,
		)
	}
	if len(output.Canonicalization.Documents) != 0 {
		t.Fatalf(
			"canonical documents = %d, want 0",
			len(output.Canonicalization.Documents),
		)
	}
	if len(output.Canonicalization.Rejected) != 1 ||
		output.Canonicalization.Rejected[0].Reason !=
			RejectionFutureAvailableAt {
		t.Fatalf(
			"rejected = %+v, want future-availability rejection",
			output.Canonicalization.Rejected,
		)
	}
	if output.Result.Status != TaskUnsatisfied {
		t.Fatalf(
			"status = %q, want %q",
			output.Result.Status,
			TaskUnsatisfied,
		)
	}
}

func TestCLSNewsRetrieverCompanyCategory(t *testing.T) {
	location := clsShanghaiLocation
	asOf := time.Date(
		2026, 9, 29, 12, 0, 0, 0, location,
	)

	client := httpDoerFunc(
		func(req *http.Request) (*http.Response, error) {
			query := req.URL.Query()
			if got := query.Get("category");
				got != "announcement" {
				t.Fatalf(
					"category = %q, want announcement",
					got,
				)
			}
			return jsonHTTPResponse(
				http.StatusOK,
				map[string]any{
					"errno": 0,
					"data": map[string]any{
						"roll_data": []map[string]any{},
					},
				},
			), nil
		},
	)

	documents, err := (ClsNewsRetriever{
		Client:   client,
		Category: CLSNewsCompany,
	}).Retrieve(
		context.Background(),
		ResearchTask{
			ID:              "task-company-news",
			InformationNeed: NeedBreakingNews,
			AsOf:            asOf,
		},
	)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(documents) != 0 {
		t.Fatalf(
			"documents = %d, want 0",
			len(documents),
		)
	}
}

func TestCLSNewsRetrieverRejectsSymbolScopedTask(
	t *testing.T,
) {
	location := clsShanghaiLocation
	asOf := time.Date(
		2026, 9, 29, 12, 0, 0, 0, location,
	)

	client := httpDoerFunc(
		func(*http.Request) (*http.Response, error) {
			t.Fatal("HTTP should not be called for symbol-scoped CLS task")
			return nil, nil
		},
	)

	output, err := (TaskRunner{}).Run(
		context.Background(),
		ResearchTask{
			ID:              "task-symbol-news",
			Symbol:          "600519.SH",
			InformationNeed: NeedBreakingNews,
			AsOf:            asOf,
			Requirements: ResearchRequirements{
				PITSafe:             true,
				RequiredSourceTypes: []SourceType{SourceIndustryMedia},
				MinDocuments:        1,
			},
		},
		ClsNewsRetriever{
			Client: client,
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output.Attempt.Status != AcquisitionFailed {
		t.Fatalf(
			"attempt status = %q, want %q",
			output.Attempt.Status,
			AcquisitionFailed,
		)
	}
	if !strings.Contains(
		output.Attempt.ErrorMessage,
		"does not support symbol-scoped task",
	) {
		t.Fatalf(
			"attempt error = %q, want symbol-scope rejection",
			output.Attempt.ErrorMessage,
		)
	}
	if len(output.RawDocuments) != 0 {
		t.Fatalf(
			"raw documents = %d, want 0",
			len(output.RawDocuments),
		)
	}
	if output.Result.Status != TaskUnsatisfied {
		t.Fatalf(
			"task status = %q, want %q",
			output.Result.Status,
			TaskUnsatisfied,
		)
	}
}

func TestCLSNewsCategoryValidation(t *testing.T) {
	if _, err := normalizeCLSNewsCategory(
		CLSNewsCategory("unknown"),
	); err == nil {
		t.Fatal("unknown category should fail")
	}

	category, err := normalizeCLSNewsCategory("")
	if err != nil {
		t.Fatalf("default category error = %v", err)
	}
	if category != CLSNewsImportant {
		t.Fatalf(
			"default category = %q, want %q",
			category,
			CLSNewsImportant,
		)
	}
}

func TestCLSQueryIsBounded(t *testing.T) {
	location := clsShanghaiLocation
	task := ResearchTask{
		ID:              "task-too-wide",
		InformationNeed: NeedBreakingNews,
		TimeRange: TimeRange{
			Start: time.Date(
				2026, 9, 1, 0, 0, 0, 0, location,
			),
			End: time.Date(
				2026, 9, 10, 0, 0, 0, 0, location,
			),
		},
	}

	_, err := (ClsNewsRetriever{
		Client: httpDoerFunc(
			func(*http.Request) (*http.Response, error) {
				t.Fatal("HTTP should not be called for unbounded query")
				return nil, nil
			},
		),
		MaxDays: 3,
	}).Retrieve(
		context.Background(),
		task,
	)
	if err == nil ||
		!strings.Contains(err.Error(), "max 3") {
		t.Fatalf(
			"error = %v, want max-days validation",
			err,
		)
	}
}



func TestCLSNewsRetrieverPreservesNumericIDAndDeduplicates(
	t *testing.T,
) {
	location := clsShanghaiLocation
	asOf := time.Date(
		2026, 9, 29, 12, 0, 0, 0, location,
	)
	published := asOf.Add(-time.Hour)

	client := httpDoerFunc(
		func(*http.Request) (*http.Response, error) {
			return jsonHTTPResponse(
				http.StatusOK,
				map[string]any{
					"errno": 0,
					"data": map[string]any{
						"roll_data": []map[string]any{
							{
								"id":      2494511,
								"title":   "重复快讯",
								"content": "正文",
								"ctime":   published.Unix(),
							},
							{
								"id":      2494511,
								"title":   "重复快讯",
								"content": "正文",
								"ctime":   published.Unix(),
							},
						},
					},
				},
			), nil
		},
	)

	documents, err := (ClsNewsRetriever{
		Client: client,
		Now: func() time.Time {
			return asOf
		},
	}).Retrieve(
		context.Background(),
		ResearchTask{
			ID:              "task-cls-numeric-id",
			InformationNeed: NeedBreakingNews,
			AsOf:            asOf,
			TimeRange: TimeRange{
				Start: clsStartOfDay(asOf),
				End:   asOf,
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"Retrieve() error = %v",
			err,
		)
	}
	if len(documents) != 1 {
		t.Fatalf(
			"documents = %d, want 1",
			len(documents),
		)
	}
	if documents[0].ID !=
		"cls_news_2494511" {
		t.Fatalf(
			"document id = %q, want exact numeric provider id",
			documents[0].ID,
		)
	}
	if documents[0].URL !=
		"https://www.cls.cn/detail/2494511" {
		t.Fatalf(
			"document url = %q",
			documents[0].URL,
		)
	}
}
