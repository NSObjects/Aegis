package event

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCninfoAnnouncementRetrieverPDFFailureKeepsMetadataAndMarksPartial(
	t *testing.T,
) {
	location := cninfoShanghaiLocation
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, location,
	)
	retrievedAt := asOf.Add(time.Minute)
	publishedAt := asOf.Add(-time.Hour)

	client := httpDoerFunc(
		func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case cninfoStockIndexURL:
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
				return jsonHTTPResponse(
					http.StatusOK,
					map[string]any{
						"totalpages": 1,
						"announcements": []map[string]any{
							{
								"announcementId":    "pdf-ok",
								"announcementTitle": "可解析公告",
								"announcementTime":  publishedAt.UnixMilli(),
								"adjunctUrl":         "finalpage/pdf-ok.PDF",
								"adjunctType":        "PDF",
								"secName":            "贵州茅台",
							},
							{
								"announcementId":    "pdf-bad",
								"announcementTitle": "损坏公告",
								"announcementTime":  publishedAt.Add(time.Minute).UnixMilli(),
								"adjunctUrl":         "finalpage/pdf-bad.PDF",
								"adjunctType":        "PDF",
								"secName":            "贵州茅台",
							},
						},
					},
				), nil

			case "https://static.cninfo.com.cn/finalpage/pdf-ok.PDF":
				return bytesHTTPResponse(
					http.StatusOK,
					[]byte("good-pdf"),
				), nil

			case "https://static.cninfo.com.cn/finalpage/pdf-bad.PDF":
				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Body: io.NopCloser(
						strings.NewReader("bad gateway"),
					),
					Header: make(http.Header),
				}, nil

			default:
				return nil, errors.New("unexpected URL")
			}
		},
	)

	store, err := NewFileEvidenceSnapshotStore(
		t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}

	task := ResearchTask{
		ID:              "cninfo-pdf-partial",
		Symbol:          "600519.SH",
		InformationNeed: NeedOfficialDisclosure,
		AsOf:            asOf,
		TimeRange: TimeRange{
			Start: asOf.Add(-24 * time.Hour),
			End:   asOf,
		},
		Requirements: ResearchRequirements{
			PITSafe:               true,
			MinDocuments:          1,
			MinIndependentSources: 1,
		},
	}

	output, err := (TaskRunner{
		Now: func() time.Time {
			return retrievedAt
		},
		SnapshotStore: store,
	}).Run(
		context.Background(),
		task,
		CninfoAnnouncementRetriever{
			Client: client,
			Now: func() time.Time {
				return retrievedAt
			},
			PDFExtractor: PDFTextExtractorFunc(
				func(
					_ context.Context,
					data []byte,
				) (PDFTextExtraction, error) {
					if string(data) != "good-pdf" {
						return PDFTextExtraction{}, errors.New(
							"unexpected PDF bytes",
						)
					}
					return PDFTextExtraction{
						Text: "  第一段\n\n第二段   重大事项  ",
					}, nil
				},
			),
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
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
		"PDF body enrichment failed for 1 announcement",
	) {
		t.Fatalf(
			"attempt error = %q",
			output.Attempt.ErrorMessage,
		)
	}
	if len(output.RawDocuments) != 2 {
		t.Fatalf(
			"raw documents = %d, want 2",
			len(output.RawDocuments),
		)
	}

	first := output.RawDocuments[0]
	if first.ID != "cninfo_announcement_pdf-ok" {
		t.Fatalf("first id = %q", first.ID)
	}
	if first.Content != "第一段 第二段 重大事项" {
		t.Fatalf("first content = %q", first.Content)
	}
	if first.RawMetadata["body_enriched"] != true {
		t.Fatalf(
			"first body_enriched = %v",
			first.RawMetadata["body_enriched"],
		)
	}
	if first.RawMetadata["body_format"] != "pdf" {
		t.Fatalf(
			"first body_format = %v",
			first.RawMetadata["body_format"],
		)
	}

	second := output.RawDocuments[1]
	if second.ID != "cninfo_announcement_pdf-bad" {
		t.Fatalf("second id = %q", second.ID)
	}
	if second.Content != "" {
		t.Fatalf(
			"failed PDF content = %q, want metadata-only",
			second.Content,
		)
	}
	if second.RawMetadata["body_enriched"] != false {
		t.Fatalf(
			"second body_enriched = %v",
			second.RawMetadata["body_enriched"],
		)
	}
	if !strings.Contains(
		second.RawMetadata["body_error"].(string),
		"502",
	) {
		t.Fatalf(
			"second body_error = %v",
			second.RawMetadata["body_error"],
		)
	}

	snapshots, err := store.Query(
		context.Background(),
		EvidenceSnapshotQuery{
			AsOf: retrievedAt,
			TimeRange: TimeRange{
				Start: asOf.Add(-24 * time.Hour),
				End:   asOf,
			},
			SourceTypes: []SourceType{
				SourceOfficial,
			},
			Symbol: "600519.SH",
		},
	)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf(
			"snapshots = %d, want 2",
			len(snapshots),
		)
	}

	foundEnriched := false
	foundMetadataOnly := false
	for _, snapshot := range snapshots {
		switch snapshot.ID {
		case "cninfo_announcement_pdf-ok":
			foundEnriched = true
			if snapshot.Content != first.Content {
				t.Fatalf(
					"snapshot content = %q, want %q",
					snapshot.Content,
					first.Content,
				)
			}
		case "cninfo_announcement_pdf-bad":
			foundMetadataOnly = true
			if snapshot.Content != "" {
				t.Fatalf(
					"failed snapshot content = %q",
					snapshot.Content,
				)
			}
		}
	}
	if !foundEnriched || !foundMetadataOnly {
		t.Fatalf(
			"snapshot identities missing: %+v",
			snapshots,
		)
	}
}

func TestDownloadCninfoPDFRejectsOversizedBody(
	t *testing.T,
) {
	client := httpDoerFunc(
		func(*http.Request) (*http.Response, error) {
			return bytesHTTPResponse(
				http.StatusOK,
				[]byte("12345"),
			), nil
		},
	)

	_, err := downloadCninfoPDF(
		context.Background(),
		client,
		"https://static.cninfo.com.cn/test.PDF",
		4,
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"4-byte limit",
		) {
		t.Fatalf(
			"oversized PDF error = %v",
			err,
		)
	}
}

func TestCninfoPDFEnrichmentBoundsTextAndKeepsIdentity(
	t *testing.T,
) {
	publishedAt := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)
	retrievedAt := publishedAt.Add(time.Hour)

	task := ResearchTask{
		ID:     "bounded-body",
		Symbol: "600519.SH",
		AsOf:   retrievedAt,
		Requirements: ResearchRequirements{
			PITSafe: true,
		},
	}
	document := cninfoRawDocument(
		task,
		cninfoStock{
			Symbol:       "600519",
			InstrumentID: "600519.SH",
			Exchange:     "SSE",
			Name:         "贵州茅台",
		},
		map[string]any{
			"announcementId":    "same-id",
			"announcementTitle": "正文截断测试",
			"announcementTime":  publishedAt.UnixMilli(),
			"adjunctUrl":         "finalpage/same-id.PDF",
			"adjunctType":        "PDF",
		},
		retrievedAt,
	)
	if document == nil {
		t.Fatal("cninfoRawDocument() returned nil")
	}
	beforeID := document.ID
	beforeURL := document.URL

	client := httpDoerFunc(
		func(*http.Request) (*http.Response, error) {
			return bytesHTTPResponse(
				http.StatusOK,
				[]byte("pdf"),
			), nil
		},
	)

	documents := []RawDocument{*document}
	err := (CninfoAnnouncementRetriever{
		PDFExtractor: PDFTextExtractorFunc(
			func(
				context.Context,
				[]byte,
			) (PDFTextExtraction, error) {
				return PDFTextExtraction{
					Text: "ABCDE",
				}, nil
			},
		),
		MaxTextChars: 3,
	}).enrichPDFBodies(
		context.Background(),
		client,
		task,
		documents,
	)
	if err != nil {
		t.Fatalf(
			"enrichPDFBodies() second error = %v",
			err,
		)
	}

	enriched := documents[0]
	if enriched.ID != beforeID ||
		enriched.URL != beforeURL {
		t.Fatalf(
			"identity changed: id=%q url=%q",
			enriched.ID,
			enriched.URL,
		)
	}
	if enriched.Content != "ABC" {
		t.Fatalf(
			"bounded content = %q, want ABC",
			enriched.Content,
		)
	}
	if enriched.RawMetadata["body_truncated"] != true {
		t.Fatalf(
			"body_truncated = %v",
			enriched.RawMetadata["body_truncated"],
		)
	}

	baseCanonical, err := (Canonicalizer{}).Canonicalize(
		task,
		[]RawDocument{*document},
	)
	if err != nil {
		t.Fatal(err)
	}
	enrichedCanonical, err := (Canonicalizer{}).Canonicalize(
		task,
		[]RawDocument{enriched},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseCanonical.Documents) != 1 ||
		len(enrichedCanonical.Documents) != 1 {
		t.Fatal("expected one canonical document")
	}
	if baseCanonical.Documents[0].ID !=
		enrichedCanonical.Documents[0].ID {
		t.Fatal(
			"full-text enrichment changed logical document identity",
		)
	}
	if baseCanonical.Documents[0].VersionID ==
		enrichedCanonical.Documents[0].VersionID {
		t.Fatal(
			"content change should create a distinct document version",
		)
	}
}


func TestCninfoPDFExtractorFailurePreservesMetadata(
	t *testing.T,
) {
	publishedAt := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)
	retrievedAt := publishedAt.Add(time.Hour)
	task := ResearchTask{
		ID:     "extractor-failure",
		Symbol: "600519.SH",
		AsOf:   retrievedAt,
	}

	document := cninfoRawDocument(
		task,
		cninfoStock{
			Symbol:       "600519",
			InstrumentID: "600519.SH",
			Exchange:     "SSE",
			Name:         "贵州茅台",
		},
		map[string]any{
			"announcementId":    "extract-fail",
			"announcementTitle": "解析失败测试",
			"announcementTime":  publishedAt.UnixMilli(),
			"adjunctUrl":         "finalpage/extract-fail.PDF",
			"adjunctType":        "PDF",
		},
		retrievedAt,
	)
	if document == nil {
		t.Fatal("cninfoRawDocument() returned nil")
	}

	client := httpDoerFunc(
		func(*http.Request) (*http.Response, error) {
			return bytesHTTPResponse(
				http.StatusOK,
				[]byte("pdf-bytes"),
			), nil
		},
	)
	documents := []RawDocument{*document}

	err := (CninfoAnnouncementRetriever{
		PDFExtractor: PDFTextExtractorFunc(
			func(
				context.Context,
				[]byte,
			) (PDFTextExtraction, error) {
				return PDFTextExtraction{},
					errors.New("extractor failed")
			},
		),
	}).enrichPDFBodies(
		context.Background(),
		client,
		task,
		documents,
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"PDF body enrichment failed for 1 announcement",
		) {
		t.Fatalf(
			"enrichPDFBodies() error = %v",
			err,
		)
	}

	got := documents[0]
	if got.Content != "" {
		t.Fatalf(
			"content = %q, want metadata-only",
			got.Content,
		)
	}
	if got.RawMetadata["body_enriched"] != false {
		t.Fatalf(
			"body_enriched = %v",
			got.RawMetadata["body_enriched"],
		)
	}
	if !strings.Contains(
		got.RawMetadata["body_error"].(string),
		"extractor failed",
	) {
		t.Fatalf(
			"body_error = %v",
			got.RawMetadata["body_error"],
		)
	}
}

func TestCninfoPDFEnrichmentStopsAfterContextCancellation(
	t *testing.T,
) {
	publishedAt := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)
	retrievedAt := publishedAt.Add(time.Hour)
	task := ResearchTask{
		ID:     "cancel-enrichment",
		Symbol: "600519.SH",
		AsOf:   retrievedAt,
	}

	makeDocument := func(
		id string,
		path string,
	) RawDocument {
		document := cninfoRawDocument(
			task,
			cninfoStock{
				Symbol:       "600519",
				InstrumentID: "600519.SH",
				Exchange:     "SSE",
				Name:         "贵州茅台",
			},
			map[string]any{
				"announcementId":    id,
				"announcementTitle": id,
				"announcementTime":  publishedAt.UnixMilli(),
				"adjunctUrl":         path,
				"adjunctType":        "PDF",
			},
			retrievedAt,
		)
		if document == nil {
			t.Fatalf(
				"cninfoRawDocument(%q) returned nil",
				id,
			)
		}
		return *document
	}

	documents := []RawDocument{
		makeDocument(
			"cancel-1",
			"finalpage/cancel-1.PDF",
		),
		makeDocument(
			"cancel-2",
			"finalpage/cancel-2.PDF",
		),
	}

	requestCount := 0
	client := httpDoerFunc(
		func(*http.Request) (*http.Response, error) {
			requestCount++
			return bytesHTTPResponse(
				http.StatusOK,
				[]byte("pdf"),
			), nil
		},
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	extractorCalls := 0
	extractor := PDFTextExtractorFunc(
		func(
			_ context.Context,
			_ []byte,
		) (PDFTextExtraction, error) {
			extractorCalls++
			cancel()
			return PDFTextExtraction{},
				context.Canceled
		},
	)

	err := (CninfoAnnouncementRetriever{
		PDFExtractor: extractor,
	}).enrichPDFBodies(
		ctx,
		client,
		task,
		documents,
	)
	if err == nil ||
		!errors.Is(err, context.Canceled) {
		t.Fatalf(
			"enrichPDFBodies() error = %v, want context.Canceled",
			err,
		)
	}
	if requestCount != 1 {
		t.Fatalf(
			"PDF requests = %d, want 1",
			requestCount,
		)
	}
	if extractorCalls != 1 {
		t.Fatalf(
			"extractor calls = %d, want 1",
			extractorCalls,
		)
	}
	if documents[1].RawMetadata["body_enriched"] != nil {
		t.Fatalf(
			"second document should remain untouched after cancellation: %+v",
			documents[1].RawMetadata,
		)
	}
}
