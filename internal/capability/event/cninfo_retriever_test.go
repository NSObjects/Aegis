package event

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(
	req *http.Request,
) (*http.Response, error) {
	return f(req)
}

func TestCninfoAnnouncementRetrieverRunsThroughTaskRunner(t *testing.T) {
	location := cninfoShanghaiLocation
	asOf := time.Date(
		2026, 9, 24, 15, 0, 0, 0, location,
	)
	before := time.Date(
		2026, 9, 24, 10, 0, 0, 0, location,
	)
	after := time.Date(
		2026, 9, 24, 16, 0, 0, 0, location,
	)
	retrievedAt := asOf.Add(24 * time.Hour)

	var seenDateRange string
	var seenStock string
	var calls int

	client := httpDoerFunc(
		func(req *http.Request) (*http.Response, error) {
			calls++
			switch req.URL.String() {
			case cninfoStockIndexURL:
				if req.Method != http.MethodGet {
					t.Fatalf(
						"stock index method = %s, want GET",
						req.Method,
					)
				}
				return jsonHTTPResponse(
					http.StatusOK,
					map[string]any{
						"stockList": []map[string]any{
							{
								"code":  "600519",
								"orgId": "gssh0600519",
								"zwjc":  "贵州茅台",
							},
						},
					},
				), nil

			case cninfoAnnouncementURL:
				if req.Method != http.MethodPost {
					t.Fatalf(
						"announcement method = %s, want POST",
						req.Method,
					)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				values, err := url.ParseQuery(
					string(body),
				)
				if err != nil {
					t.Fatalf("parse request body: %v", err)
				}
				seenDateRange = values.Get("seDate")
				seenStock = values.Get("stock")
				if got := values.Get("column"); got != "sse" {
					t.Fatalf(
						"column = %q, want sse",
						got,
					)
				}
				if got := values.Get("plate"); got != "sh" {
					t.Fatalf(
						"plate = %q, want sh",
						got,
					)
				}

				return jsonHTTPResponse(
					http.StatusOK,
					map[string]any{
						"totalpages": 1,
						"announcements": []map[string]any{
							{
								"announcementId":    "1220000001",
								"announcementTitle": "<em>贵州茅台</em> 2026年公告",
								"announcementTime":  before.UnixMilli(),
								"adjunctUrl":         "finalpage/2026-09-24/1220000001.PDF",
								"adjunctType":        "PDF",
								"adjunctSize":        156.5,
								"secName":            "贵州茅台",
							},
							{
								"announcementId":    "1220000002",
								"announcementTitle": "收盘后公告",
								"announcementTime":  after.UnixMilli(),
								"adjunctUrl":         "finalpage/2026-09-24/1220000002.PDF",
								"adjunctType":        "PDF",
								"adjunctSize":        88.0,
								"secName":            "贵州茅台",
							},
						},
					},
				), nil
			default:
				t.Fatalf(
					"unexpected URL: %s",
					req.URL.String(),
				)
				return nil, nil
			}
		},
	)

	task := ResearchTask{
		ID:              "task-cninfo",
		Symbol:          "600519.SH",
		InformationNeed: NeedOfficialDisclosure,
		AsOf:            asOf,
		TimeRange: TimeRange{
			Start: time.Date(
				2026, 9, 23, 0, 0, 0, 0, location,
			),
			End: time.Date(
				2026, 9, 25, 0, 0, 0, 0, location,
			),
		},
		Requirements: ResearchRequirements{
			PITSafe:             true,
			RequiredSourceTypes: []SourceType{SourceOfficial},
			RequiredAuthorities: []EvidenceAuthority{
				AuthorityIssuer,
			},
			RequiredEvidenceKinds: []EvidenceKind{
				EvidenceDisclosure,
			},
			MinDocuments:          1,
			MinIndependentSources: 1,
			RequirePrimarySource:  true,
		},
	}

	runner := TaskRunner{
		Now: func() time.Time {
			return retrievedAt
		},
	}
	output, err := runner.Run(
		context.Background(),
		task,
		CninfoAnnouncementRetriever{
			Client: client,
			Now: func() time.Time {
				return retrievedAt
			},
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if calls != 2 {
		t.Fatalf("HTTP calls = %d, want 2", calls)
	}
	if seenDateRange != "2026-09-23~2026-09-24" {
		t.Fatalf(
			"seDate = %q, want clipped as_of date range",
			seenDateRange,
		)
	}
	if seenStock != "600519,gssh0600519" {
		t.Fatalf(
			"stock form value = %q",
			seenStock,
		)
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
	if raw.ID != "cninfo_announcement_1220000001" {
		t.Fatalf("raw id = %q", raw.ID)
	}
	if raw.Title != "贵州茅台 2026年公告" {
		t.Fatalf("raw title = %q", raw.Title)
	}
	if raw.Publisher != cninfoPublisher {
		t.Fatalf("publisher = %q", raw.Publisher)
	}
	if raw.SourceType != SourceOfficial {
		t.Fatalf("source type = %q", raw.SourceType)
	}
	if raw.Authority != AuthorityIssuer {
		t.Fatalf("authority = %q", raw.Authority)
	}
	if raw.EvidenceKind != EvidenceDisclosure {
		t.Fatalf("evidence kind = %q", raw.EvidenceKind)
	}
	if !raw.IsPrimary {
		t.Fatal("Cninfo announcement should be primary evidence")
	}
	if raw.URL != "https://static.cninfo.com.cn/finalpage/2026-09-24/1220000001.PDF" {
		t.Fatalf("raw URL = %q", raw.URL)
	}
	if raw.PublishedAt == nil ||
		!raw.PublishedAt.Equal(before) {
		t.Fatalf(
			"published_at = %v, want %s",
			raw.PublishedAt,
			before,
		)
	}
	if raw.RawMetadata["instrument_id"] != "600519.SH" {
		t.Fatalf(
			"instrument_id metadata = %v",
			raw.RawMetadata["instrument_id"],
		)
	}
	if raw.AttemptID != output.Attempt.ID {
		t.Fatalf(
			"attempt id = %q, want %q",
			raw.AttemptID,
			output.Attempt.ID,
		)
	}
}

func TestCninfoAnnouncementRetrieverPreservesDocumentsOnLaterPageFailure(
	t *testing.T,
) {
	location := cninfoShanghaiLocation
	asOf := time.Date(
		2026, 9, 24, 23, 59, 0, 0, location,
	)
	publishedAt := asOf.Add(-time.Hour)
	retrievedAt := asOf.Add(time.Hour)
	announcementPageCalls := 0

	client := httpDoerFunc(
		func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case cninfoStockIndexURL:
				return jsonHTTPResponse(
					http.StatusOK,
					map[string]any{
						"stockList": []map[string]any{
							{
								"code":  "000001",
								"orgId": "gssz0000001",
								"zwjc":  "平安银行",
							},
						},
					},
				), nil

			case cninfoAnnouncementURL:
				announcementPageCalls++
				if announcementPageCalls == 1 {
					return jsonHTTPResponse(
						http.StatusOK,
						map[string]any{
							"totalpages": 2,
							"announcements": []map[string]any{
								{
									"announcementId":    "1210000001",
									"announcementTitle": "平安银行公告",
									"announcementTime":  publishedAt.UnixMilli(),
									"adjunctUrl":         "finalpage/2026-09-24/1210000001.PDF",
									"secName":            "平安银行",
								},
							},
						},
					), nil
				}
				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Body: io.NopCloser(
						strings.NewReader("upstream failed"),
					),
					Header: make(http.Header),
				}, nil
			default:
				return nil, errors.New("unexpected URL")
			}
		},
	)

	task := ResearchTask{
		ID:              "task-cninfo-partial",
		Symbol:          "000001.SZ",
		InformationNeed: NeedOfficialDisclosure,
		AsOf:            asOf,
		TimeRange: TimeRange{
			Start: asOf.Add(-7 * 24 * time.Hour),
			End:   asOf,
		},
		Requirements: ResearchRequirements{
			PITSafe:               true,
			RequiredSourceTypes:   []SourceType{SourceOfficial},
			MinDocuments:          1,
			MinIndependentSources: 1,
			RequirePrimarySource:  true,
		},
	}

	output, err := (TaskRunner{
		Now: func() time.Time {
			return retrievedAt
		},
	}).Run(
		context.Background(),
		task,
		CninfoAnnouncementRetriever{
			Client:   client,
			PageSize: 1,
			MaxPages: 2,
			Now: func() time.Time {
				return retrievedAt
			},
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if announcementPageCalls != 2 {
		t.Fatalf(
			"announcement calls = %d, want 2",
			announcementPageCalls,
		)
	}
	if output.Attempt.Status != AcquisitionPartial {
		t.Fatalf(
			"attempt status = %q, want %q",
			output.Attempt.Status,
			AcquisitionPartial,
		)
	}
	if !strings.Contains(
		output.Attempt.ErrorMessage,
		"page 2",
	) {
		t.Fatalf(
			"attempt error = %q, want page 2 context",
			output.Attempt.ErrorMessage,
		)
	}
	if len(output.RawDocuments) != 1 {
		t.Fatalf(
			"raw documents = %d, want 1",
			len(output.RawDocuments),
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
	if output.RawDocuments[0].RawMetadata["instrument_id"] != "000001.SZ" {
		t.Fatalf(
			"instrument id = %v",
			output.RawDocuments[0].RawMetadata["instrument_id"],
		)
	}
}

func TestNormalizeCninfoSymbol(t *testing.T) {
	tests := map[string]string{
		"600519.SH": "600519",
		"sh600519":  "600519",
		"000001.SZ": "000001",
		"sz000001":  "000001",
		"920001.BJ": "920001",
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := normalizeCninfoSymbol(input)
			if err != nil {
				t.Fatalf(
					"normalizeCninfoSymbol() error = %v",
					err,
				)
			}
			if got != want {
				t.Fatalf(
					"symbol = %q, want %q",
					got,
					want,
				)
			}
		})
	}

	if _, err := normalizeCninfoSymbol(
		"not-a-symbol",
	); err == nil {
		t.Fatal("invalid symbol should fail")
	}
}

func jsonHTTPResponse(
	status int,
	payload any,
) *http.Response {
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return &http.Response{
		StatusCode: status,
		Body: io.NopCloser(
			strings.NewReader(string(data)),
		),
		Header: make(http.Header),
	}
}
