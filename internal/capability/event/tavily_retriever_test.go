package event

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTavilyWebSearchRetrieverUsesFullContentAndTargetQuery(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 16, 0, 0, 0, time.UTC,
	)
	start := asOf.Add(-48 * time.Hour)

	var captured tavilySearchRequest
	client := httpDoerFunc(
		func(req *http.Request) (*http.Response, error) {
			if req.Header.Get(
				"X-Tavily-Access-Mode",
			) != "keyless" {
				t.Fatalf(
					"keyless header = %q",
					req.Header.Get(
						"X-Tavily-Access-Mode",
					),
				)
			}
			if req.Header.Get("Authorization") != "" {
				t.Fatalf(
					"unexpected authorization = %q",
					req.Header.Get("Authorization"),
				)
			}
			if err := json.NewDecoder(
				req.Body,
			).Decode(&captured); err != nil {
				t.Fatalf(
					"decode request: %v",
					err,
				)
			}

			return bytesHTTPResponse(
				http.StatusOK,
				[]byte(`{
					"query":"ignored",
					"results":[
						{
							"id":"result-1",
							"title":"贵州茅台出现重要经营进展",
							"url":"https://news.example.com/a#section",
							"content":"THIS IS ONLY A SEARCH SNIPPET",
							"raw_content":"完整 网页 正文 贵州茅台 600519 经营事件",
							"score":0.94,
							"published_date":"Mon, 29 Sep 2026 10:00:00 GMT"
						}
					],
					"request_id":"request-1"
				}`),
			), nil
		},
	)

	retriever := TavilyWebSearchRetriever{
		Client: client,
		Now: func() time.Time {
			return asOf.Add(time.Minute)
		},
		TargetResolver: MapEvidenceTargetResolver{
			"600519.SH": {
				Symbol: "600519.SH",
				Name:   "贵州茅台",
				Aliases: []string{
					"600519",
				},
			},
		},
	}

	documents, err := retriever.Retrieve(
		context.Background(),
		ResearchTask{
			ID:              "tavily-target",
			TargetSymbol:    "600519.SH",
			InformationNeed: NeedBreakingNews,
			AsOf:            asOf,
			TimeRange: TimeRange{
				Start: start,
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

	document := documents[0]
	if strings.Contains(
		document.Content,
		"SEARCH SNIPPET",
	) {
		t.Fatalf(
			"snippet leaked into evidence content: %q",
			document.Content,
		)
	}
	if document.Content !=
		"完整 网页 正文 贵州茅台 600519 经营事件" {
		t.Fatalf(
			"content = %q",
			document.Content,
		)
	}
	if document.URL !=
		"https://news.example.com/a" {
		t.Fatalf(
			"url = %q",
			document.URL,
		)
	}
	if document.Publisher !=
		"news.example.com" {
		t.Fatalf(
			"publisher = %q",
			document.Publisher,
		)
	}
	if document.SourceType != SourceOther ||
		document.Authority != "" ||
		document.EvidenceKind != EvidenceOther ||
		document.IsPrimary {
		t.Fatalf(
			"evidence semantics = source:%q authority:%q kind:%q primary:%v",
			document.SourceType,
			document.Authority,
			document.EvidenceKind,
			document.IsPrimary,
		)
	}
	if document.RawMetadata["search_snippet"] !=
		"THIS IS ONLY A SEARCH SNIPPET" {
		t.Fatalf(
			"search_snippet = %v",
			document.RawMetadata["search_snippet"],
		)
	}

	if captured.Topic != "news" ||
		captured.SearchDepth != "advanced" ||
		captured.IncludeRawContent != "text" ||
		!captured.IncludePublishedDate ||
		captured.IncludeAnswer {
		t.Fatalf(
			"request = %+v",
			captured,
		)
	}
	if !strings.Contains(
		captured.Query,
		"贵州茅台",
	) ||
		!strings.Contains(
			captured.Query,
			"600519",
		) {
		t.Fatalf(
			"target-aware query = %q",
			captured.Query,
		)
	}
	if captured.StartDate !=
		tavilyDate(start) ||
		captured.EndDate !=
			tavilyDate(asOf) {
		t.Fatalf(
			"request dates = %q..%q",
			captured.StartDate,
			captured.EndDate,
		)
	}
}

func TestTavilyWebSearchRetrieverUsesBearerWhenConfigured(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 16, 0, 0, 0, time.UTC,
	)

	client := httpDoerFunc(
		func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Authorization") !=
				"Bearer tvly-test" {
				t.Fatalf(
					"authorization = %q",
					req.Header.Get(
						"Authorization",
					),
				)
			}
			if req.Header.Get(
				"X-Tavily-Access-Mode",
			) != "" {
				t.Fatalf(
					"unexpected keyless header = %q",
					req.Header.Get(
						"X-Tavily-Access-Mode",
					),
				)
			}
			return bytesHTTPResponse(
				http.StatusOK,
				[]byte(`{"results":[]}`),
			), nil
		},
	)

	_, err := (TavilyWebSearchRetriever{
		Client: client,
		APIKey: "tvly-test",
		Now: func() time.Time {
			return asOf
		},
	}).Retrieve(
		context.Background(),
		ResearchTask{
			ID:              "tavily-keyed",
			TargetSymbol:    "600519.SH",
			InformationNeed: NeedBreakingNews,
			AsOf:            asOf,
		},
	)
	if err != nil {
		t.Fatalf(
			"Retrieve() error = %v",
			err,
		)
	}
}

func TestTavilyWebSearchPartialCoverageBecomesPartialAttempt(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 16, 0, 0, 0, time.UTC,
	)
	published := "Mon, 29 Sep 2026 10:00:00 GMT"

	client := httpDoerFunc(
		func(*http.Request) (*http.Response, error) {
			return bytesHTTPResponse(
				http.StatusOK,
				[]byte(`{
					"results":[
						{
							"id":"good",
							"title":"good",
							"url":"https://good.example/a",
							"content":"snippet-good",
							"raw_content":"usable full content 600519",
							"published_date":"`+published+`"
						},
						{
							"id":"snippet-only",
							"title":"bad",
							"url":"https://bad.example/b",
							"content":"snippet must not become evidence",
							"raw_content":"",
							"published_date":"`+published+`"
						}
					]
				}`),
			), nil
		},
	)

	output, err := (TaskRunner{
		Now: func() time.Time {
			return asOf.Add(time.Minute)
		},
	}).Run(
		context.Background(),
		ResearchTask{
			ID:              "tavily-partial",
			TargetSymbol:    "600519.SH",
			InformationNeed: NeedBreakingNews,
			AsOf:            asOf,
			TimeRange: TimeRange{
				Start: asOf.Add(-24 * time.Hour),
				End:   asOf,
			},
		},
		TavilyWebSearchRetriever{
			Client: client,
			Now: func() time.Time {
				return asOf.Add(time.Minute)
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"TaskRunner.Run() structural error = %v",
			err,
		)
	}
	if output.Attempt.Status !=
		AcquisitionPartial {
		t.Fatalf(
			"attempt status = %q, want %q",
			output.Attempt.Status,
			AcquisitionPartial,
		)
	}
	if len(output.RawDocuments) != 1 {
		t.Fatalf(
			"raw documents = %d, want 1",
			len(output.RawDocuments),
		)
	}
	if !strings.Contains(
		output.Attempt.ErrorMessage,
		"missing raw_content",
	) {
		t.Fatalf(
			"attempt error = %q",
			output.Attempt.ErrorMessage,
		)
	}
	if strings.Contains(
		output.RawDocuments[0].Content,
		"snippet",
	) {
		t.Fatalf(
			"snippet became evidence: %q",
			output.RawDocuments[0].Content,
		)
	}
}

func TestTavilyWebSearchRequiresCandidateTargetContext(
	t *testing.T,
) {
	retriever := TavilyWebSearchRetriever{
		Client: httpDoerFunc(
			func(*http.Request) (*http.Response, error) {
				return nil, errors.New(
					"should not request",
				)
			},
		),
	}

	tests := []struct {
		name string
		task ResearchTask
		want string
	}{
		{
			name: "symbol-scoped candidate",
			task: ResearchTask{
				Symbol:          "600519.SH",
				TargetSymbol:    "600519.SH",
				InformationNeed: NeedBreakingNews,
			},
			want: "expects candidate task Symbol to be empty",
		},
		{
			name: "missing target symbol",
			task: ResearchTask{
				InformationNeed: NeedBreakingNews,
			},
			want: "target symbol is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := retriever.Retrieve(
				context.Background(),
				test.task,
			)
			if err == nil ||
				!strings.Contains(
					err.Error(),
					test.want,
				) {
				t.Fatalf(
					"error = %v, want %q",
					err,
					test.want,
				)
			}
		})
	}
}

func TestTavilyDocumentIDStableAcrossSearchResultIDs(
	t *testing.T,
) {
	first := shortID(
		"tavily_web",
		"https://news.example.com/a",
	)
	second := shortID(
		"tavily_web",
		"https://news.example.com/a",
	)
	if first != second {
		t.Fatalf(
			"stable ids differ: %q vs %q",
			first,
			second,
		)
	}
}
