package event

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakeEvidenceSelector struct {
	selectFn func(
		context.Context,
		ResearchTask,
		[]RawDocument,
	) ([]EvidenceSelectionDecision, error)
}

func (s fakeEvidenceSelector) Select(
	ctx context.Context,
	task ResearchTask,
	documents []RawDocument,
) ([]EvidenceSelectionDecision, error) {
	return s.selectFn(ctx, task, documents)
}

type countingRetriever struct {
	id       string
	method   AcquisitionMethod
	docs     []RawDocument
	err      error
	callCount *int
}

func (r countingRetriever) ID() string {
	return r.id
}

func (r countingRetriever) Method() AcquisitionMethod {
	return r.method
}

func (r countingRetriever) Retrieve(
	_ context.Context,
	_ ResearchTask,
) ([]RawDocument, error) {
	if r.callCount != nil {
		*r.callCount++
	}
	documents := append(
		[]RawDocument(nil),
		r.docs...,
	)
	for index := range documents {
		if documents[index].AvailableAt == nil &&
			documents[index].PublishedAt != nil {
			documents[index].AvailableAt = cloneTime(
				documents[index].PublishedAt,
			)
		}
	}
	return documents, r.err
}

func TestAcquisitionPlanRunnerWithCninfoAndCLS(t *testing.T) {
	location := clsShanghaiLocation
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, location,
	)
	retrievedAt := asOf.Add(-time.Minute)
	cninfoPublishedAt := time.Date(
		2026, 9, 29, 10, 0, 0, 0, location,
	)
	clsRelevantAt := time.Date(
		2026, 9, 29, 11, 0, 0, 0, location,
	)
	clsIrrelevantAt := time.Date(
		2026, 9, 29, 12, 0, 0, 0, location,
	)

	cninfoClient := httpDoerFunc(
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
			case "https://static.cninfo.com.cn/finalpage/2026-09-29/1227000001.PDF":
				return bytesHTTPResponse(
					http.StatusOK,
					[]byte("fake-plan-pdf"),
				), nil

			case cninfoAnnouncementURL:
				return jsonHTTPResponse(
					http.StatusOK,
					map[string]any{
						"totalpages": 1,
						"announcements": []map[string]any{
							{
								"announcementId":    "1227000001",
								"announcementTitle": "贵州茅台公告",
								"announcementTime":  cninfoPublishedAt.UnixMilli(),
								"adjunctUrl":         "finalpage/2026-09-29/1227000001.PDF",
								"secName":            "贵州茅台",
							},
						},
					},
				), nil
			default:
				t.Fatalf(
					"unexpected Cninfo URL: %s",
					req.URL.String(),
				)
				return nil, nil
			}
		},
	)

	clsClient := httpDoerFunc(
		func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/v1/roll/get_roll_list" {
				t.Fatalf(
					"unexpected CLS path: %s",
					req.URL.Path,
				)
			}
			return jsonHTTPResponse(
				http.StatusOK,
				map[string]any{
					"errno": 0,
					"data": map[string]any{
						"roll_data": []map[string]any{
							{
								"id":      "2457001",
								"title":   "贵州茅台盘中消息",
								"content": "与目标公司相关的候选新闻",
								"ctime":   clsRelevantAt.Unix(),
							},
							{
								"id":      "2457002",
								"title":   "其他行业消息",
								"content": "与目标公司无关的候选新闻",
								"ctime":   clsIrrelevantAt.Unix(),
							},
						},
					},
				},
			), nil
		},
	)

	root := ResearchTask{
		ID:              "task-event-evidence",
		Symbol:          "600519.SH",
		Objective:       "collect event evidence",
		InformationNeed: NeedEventEvidence,
		AsOf:            asOf,
		TimeRange: TimeRange{
			Start: clsStartOfDay(asOf),
			End:   asOf.Add(3 * time.Hour),
		},
		Requirements: ResearchRequirements{
			PITSafe: true,
			RequiredSourceTypes: []SourceType{
				SourceOfficial,
				SourceIndustryMedia,
			},
			MinDocuments:          2,
			MinIndependentSources: 2,
			RequirePrimarySource:  true,
		},
	}

	selector := fakeEvidenceSelector{
		selectFn: func(
			_ context.Context,
			task ResearchTask,
			documents []RawDocument,
		) ([]EvidenceSelectionDecision, error) {
			if task.Symbol != "600519.SH" {
				t.Fatalf(
					"selector root symbol = %q",
					task.Symbol,
				)
			}
			if len(documents) != 2 {
				t.Fatalf(
					"selector candidates = %d, want 2",
					len(documents),
				)
			}
			return []EvidenceSelectionDecision{
				{
					RawDocumentID: "cls_news_2457001",
					Selected:      true,
					Reason:        "target company match",
				},
				{
					RawDocumentID: "cls_news_2457002",
					Selected:      false,
					Reason:        "unrelated company",
				},
			}, nil
		},
	}

	output, err := (AcquisitionPlanRunner{
		Now: func() time.Time {
			return retrievedAt
		},
	}).Run(
		context.Background(),
		root,
		AcquisitionPlan{
			Steps: []AcquisitionStep{
				{
					ID:    "official-disclosure",
					Scope: AcquisitionScopeDirect,
					Task: ResearchTask{
						Symbol:          "600519.SH",
						InformationNeed: NeedOfficialDisclosure,
					},
					Retriever: CninfoAnnouncementRetriever{
						Client: cninfoClient,
						Now: func() time.Time {
							return retrievedAt
						},
						PDFExtractor: PDFTextExtractorFunc(
							func(
								context.Context,
								[]byte,
							) (PDFTextExtraction, error) {
								return PDFTextExtraction{
									Text: "贵州茅台公告正文",
								}, nil
							},
						),
					},
				},
				{
					ID:    "breaking-news",
					Scope: AcquisitionScopeCandidate,
					Task: ResearchTask{
						InformationNeed: NeedBreakingNews,
					},
					Retriever: ClsNewsRetriever{
						Client: clsClient,
						Now: func() time.Time {
							return retrievedAt
						},
						Category: CLSNewsImportant,
						MaxDays:  1,
					},
				},
			},
			Selector: selector,
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(output.Steps) != 2 {
		t.Fatalf(
			"step outputs = %d, want 2",
			len(output.Steps),
		)
	}
	if output.Steps[0].Task.ID != root.ID ||
		output.Steps[1].Task.ID != root.ID {
		t.Fatalf(
			"step task ids = %q, %q; want %q",
			output.Steps[0].Task.ID,
			output.Steps[1].Task.ID,
			root.ID,
		)
	}
	if output.Steps[0].Task.Symbol != "600519.SH" {
		t.Fatalf(
			"Cninfo step symbol = %q",
			output.Steps[0].Task.Symbol,
		)
	}
	if output.Steps[1].Task.Symbol != "" {
		t.Fatalf(
			"CLS candidate step symbol = %q, want empty",
			output.Steps[1].Task.Symbol,
		)
	}
	if !output.Steps[0].Task.Requirements.PITSafe ||
		!output.Steps[1].Task.Requirements.PITSafe {
		t.Fatal("root PIT safety must propagate to every acquisition step")
	}
	if len(output.CandidateRawDocuments) != 2 {
		t.Fatalf(
			"candidate raw documents = %d, want 2",
			len(output.CandidateRawDocuments),
		)
	}
	if len(output.Selections) != 2 {
		t.Fatalf(
			"selection decisions = %d, want 2",
			len(output.Selections),
		)
	}
	if len(output.AcceptedRawDocuments) != 2 {
		t.Fatalf(
			"accepted raw documents = %d, want 2",
			len(output.AcceptedRawDocuments),
		)
	}
	if len(output.Canonicalization.Documents) != 2 {
		t.Fatalf(
			"canonical documents = %d, want 2",
			len(output.Canonicalization.Documents),
		)
	}
	if len(output.Result.Attempts) != 2 {
		t.Fatalf(
			"root attempts = %d, want 2",
			len(output.Result.Attempts),
		)
	}
	if output.Result.Status != TaskSatisfied {
		t.Fatalf(
			"root status = %q, want %q; gaps=%v",
			output.Result.Status,
			TaskSatisfied,
			output.Result.Gaps,
		)
	}

	var hasOfficial, hasIndustry bool
	for _, document := range output.Canonicalization.Documents {
		switch document.Source.SourceType {
		case SourceOfficial:
			hasOfficial = true
		case SourceIndustryMedia:
			hasIndustry = true
		}
	}
	if !hasOfficial || !hasIndustry {
		t.Fatalf(
			"canonical source types official=%v industry=%v",
			hasOfficial,
			hasIndustry,
		)
	}
}

func TestAcquisitionPlanRunnerRequiresSelectorForCandidates(
	t *testing.T,
) {
	publishedAt := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)

	output, err := (AcquisitionPlanRunner{}).Run(
		context.Background(),
		ResearchTask{
			ID: "task-candidate-no-selector",
		},
		AcquisitionPlan{
			Steps: []AcquisitionStep{
				{
					ID:    "candidate",
					Scope: AcquisitionScopeCandidate,
					Task: ResearchTask{
						InformationNeed: NeedBreakingNews,
					},
					Retriever: fakeRetriever{
						id:     "candidate_api",
						method: AcquisitionAPI,
						docs: []RawDocument{
							{
								ID:          "candidate-1",
								Title:       "candidate",
								Publisher:   "media",
								SourceType:  SourceIndustryMedia,
								PublishedAt: &publishedAt,
							},
						},
					},
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"selector is required",
		) {
		t.Fatalf(
			"error = %v, want selector-required error",
			err,
		)
	}
	if len(output.CandidateRawDocuments) != 1 {
		t.Fatalf(
			"candidate raw documents = %d, want 1",
			len(output.CandidateRawDocuments),
		)
	}
	if len(output.AcceptedRawDocuments) != 0 {
		t.Fatalf(
			"accepted raw documents = %d, want 0",
			len(output.AcceptedRawDocuments),
		)
	}
	if len(output.Canonicalization.Documents) != 0 {
		t.Fatal("candidate documents must not reach root canonicalization")
	}
}

func TestApplyEvidenceSelectionValidation(t *testing.T) {
	candidates := []RawDocument{
		{ID: "candidate-a"},
		{ID: "candidate-b"},
	}

	tests := []struct {
		name      string
		decisions []EvidenceSelectionDecision
		want      string
	}{
		{
			name: "unknown document",
			decisions: []EvidenceSelectionDecision{
				{
					RawDocumentID: "candidate-a",
					Selected:      true,
				},
				{
					RawDocumentID: "candidate-c",
					Selected:      false,
				},
			},
			want: "unknown raw document",
		},
		{
			name: "duplicate decision",
			decisions: []EvidenceSelectionDecision{
				{
					RawDocumentID: "candidate-a",
					Selected:      true,
				},
				{
					RawDocumentID: "candidate-a",
					Selected:      false,
				},
			},
			want: "duplicated",
		},
		{
			name: "missing decision",
			decisions: []EvidenceSelectionDecision{
				{
					RawDocumentID: "candidate-a",
					Selected:      true,
				},
			},
			want: "missing",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := applyEvidenceSelection(
				candidates,
				test.decisions,
			)
			if err == nil ||
				!strings.Contains(
					err.Error(),
					test.want,
				) {
				t.Fatalf(
					"error = %v, want substring %q",
					err,
					test.want,
				)
			}
		})
	}
}

func TestAcquisitionPlanRunnerExecutesAllStepsAfterRetrieverFailure(
	t *testing.T,
) {
	publishedAt := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)
	officialCalls := 0
	mediaCalls := 0

	output, err := (AcquisitionPlanRunner{}).Run(
		context.Background(),
		ResearchTask{
			ID: "task-source-failure",
			Requirements: ResearchRequirements{
				RequiredSourceTypes: []SourceType{
					SourceOfficial,
					SourceIndustryMedia,
				},
				MinDocuments: 2,
			},
		},
		AcquisitionPlan{
			Steps: []AcquisitionStep{
				{
					ID:    "official",
					Scope: AcquisitionScopeDirect,
					Retriever: countingRetriever{
						id:        "official_api",
						method:    AcquisitionAPI,
						err:       errors.New("official source unavailable"),
						callCount: &officialCalls,
					},
				},
				{
					ID:    "media",
					Scope: AcquisitionScopeDirect,
					Retriever: countingRetriever{
						id:     "media_api",
						method: AcquisitionAPI,
						docs: []RawDocument{
							{
								ID:          "media-1",
								Title:       "media evidence",
								URL:         "https://media.example/1",
								Publisher:   "media",
								SourceType:  SourceIndustryMedia,
								PublishedAt: &publishedAt,
							},
						},
						callCount: &mediaCalls,
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if officialCalls != 1 || mediaCalls != 1 {
		t.Fatalf(
			"calls official=%d media=%d, want 1/1",
			officialCalls,
			mediaCalls,
		)
	}
	if len(output.Steps) != 2 {
		t.Fatalf(
			"steps = %d, want 2",
			len(output.Steps),
		)
	}
	if output.Steps[0].Run.Attempt.Status != AcquisitionFailed {
		t.Fatalf(
			"official attempt status = %q",
			output.Steps[0].Run.Attempt.Status,
		)
	}
	if output.Steps[1].Run.Attempt.Status != AcquisitionSucceeded {
		t.Fatalf(
			"media attempt status = %q",
			output.Steps[1].Run.Attempt.Status,
		)
	}
	if len(output.Result.Attempts) != 2 {
		t.Fatalf(
			"root attempts = %d, want 2",
			len(output.Result.Attempts),
		)
	}
	if output.Result.Status != TaskPartial {
		t.Fatalf(
			"root status = %q, want %q; gaps=%v",
			output.Result.Status,
			TaskPartial,
			output.Result.Gaps,
		)
	}
}

func TestAcquisitionPlanRunnerDoesNotExposeFutureCandidateToSelector(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 12, 0, 0, 0, time.UTC,
	)
	before := asOf.Add(-time.Hour)
	after := asOf.Add(time.Hour)
	selectorCalls := 0

	output, err := (AcquisitionPlanRunner{}).Run(
		context.Background(),
		ResearchTask{
			ID:   "task-pit-candidate",
			AsOf: asOf,
			Requirements: ResearchRequirements{
				PITSafe:             true,
				RequiredSourceTypes: []SourceType{SourceIndustryMedia},
				MinDocuments:        1,
			},
		},
		AcquisitionPlan{
			Steps: []AcquisitionStep{
				{
					ID:    "candidate",
					Scope: AcquisitionScopeCandidate,
					Task: ResearchTask{
						InformationNeed: NeedBreakingNews,
					},
					Retriever: fakeRetriever{
						id:     "candidate_api",
						method: AcquisitionAPI,
						docs: []RawDocument{
							{
								ID:          "before",
								Title:       "before",
								URL:         "https://media.example/before",
								Publisher:   "media",
								SourceType:  SourceIndustryMedia,
								PublishedAt: &before,
							},
							{
								ID:          "after",
								Title:       "after",
								URL:         "https://media.example/after",
								Publisher:   "media",
								SourceType:  SourceIndustryMedia,
								PublishedAt: &after,
							},
						},
					},
				},
			},
			Selector: fakeEvidenceSelector{
				selectFn: func(
					_ context.Context,
					_ ResearchTask,
					documents []RawDocument,
				) ([]EvidenceSelectionDecision, error) {
					selectorCalls++
					if len(documents) != 1 ||
						documents[0].ID != "before" {
						t.Fatalf(
							"selector documents = %+v; want only PIT-safe candidate",
							documents,
						)
					}
					return []EvidenceSelectionDecision{
						{
							RawDocumentID: "before",
							Selected:      true,
							Reason:        "relevant",
						},
					}, nil
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if selectorCalls != 1 {
		t.Fatalf(
			"selector calls = %d, want 1",
			selectorCalls,
		)
	}
	if len(output.CandidateRawDocuments) != 1 {
		t.Fatalf(
			"candidate documents = %d, want 1",
			len(output.CandidateRawDocuments),
		)
	}
	if output.Result.Status != TaskSatisfied {
		t.Fatalf(
			"root status = %q, want %q",
			output.Result.Status,
			TaskSatisfied,
		)
	}
}

func TestAcquisitionPlanRunnerCanonicalizesAcrossSteps(
	t *testing.T,
) {
	publishedAt := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)
	const sharedURL = "https://example.com/shared"

	output, err := (AcquisitionPlanRunner{}).Run(
		context.Background(),
		ResearchTask{
			ID: "task-cross-source-canonicalization",
			Requirements: ResearchRequirements{
				MinDocuments: 1,
			},
		},
		AcquisitionPlan{
			Steps: []AcquisitionStep{
				{
					ID:    "direct",
					Scope: AcquisitionScopeDirect,
					Retriever: fakeRetriever{
						id:     "direct_api",
						method: AcquisitionAPI,
						docs: []RawDocument{
							{
								ID:          "raw-direct",
								Title:       "shared",
								URL:         sharedURL,
								Publisher:   "source-a",
								SourceType:  SourceOfficial,
								PublishedAt: &publishedAt,
							},
						},
					},
				},
				{
					ID:    "candidate",
					Scope: AcquisitionScopeCandidate,
					Retriever: fakeRetriever{
						id:     "candidate_api",
						method: AcquisitionAPI,
						docs: []RawDocument{
							{
								ID:          "raw-candidate",
								Title:       "shared copy",
								URL:         sharedURL,
								Publisher:   "source-b",
								SourceType:  SourceIndustryMedia,
								PublishedAt: &publishedAt,
							},
						},
					},
				},
			},
			Selector: fakeEvidenceSelector{
				selectFn: func(
					_ context.Context,
					_ ResearchTask,
					_ []RawDocument,
				) ([]EvidenceSelectionDecision, error) {
					return []EvidenceSelectionDecision{
						{
							RawDocumentID: "raw-candidate",
							Selected:      true,
						},
					}, nil
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(output.AcceptedRawDocuments) != 2 {
		t.Fatalf(
			"accepted raw documents = %d, want 2",
			len(output.AcceptedRawDocuments),
		)
	}
	if len(output.Canonicalization.Documents) != 1 {
		t.Fatalf(
			"canonical documents = %d, want 1",
			len(output.Canonicalization.Documents),
		)
	}
	if len(
		output.Canonicalization.Documents[0].RawDocumentIDs,
	) != 2 {
		t.Fatalf(
			"merged raw ids = %v, want 2",
			output.Canonicalization.Documents[0].RawDocumentIDs,
		)
	}
}

func TestAcquisitionPlanRunnerReturnsSelectorError(t *testing.T) {
	publishedAt := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)

	output, err := (AcquisitionPlanRunner{}).Run(
		context.Background(),
		ResearchTask{
			ID: "task-selector-error",
		},
		AcquisitionPlan{
			Steps: []AcquisitionStep{
				{
					ID:    "candidate",
					Scope: AcquisitionScopeCandidate,
					Retriever: fakeRetriever{
						id:     "candidate_api",
						method: AcquisitionAPI,
						docs: []RawDocument{
							{
								ID:          "candidate-selector-error",
								Title:       "candidate",
								URL:         "https://media.example/selector-error",
								Publisher:   "media",
								SourceType:  SourceIndustryMedia,
								PublishedAt: &publishedAt,
							},
						},
					},
				},
			},
			Selector: fakeEvidenceSelector{
				selectFn: func(
					_ context.Context,
					_ ResearchTask,
					_ []RawDocument,
				) ([]EvidenceSelectionDecision, error) {
					return nil, errors.New("selector unavailable")
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"selector unavailable",
		) {
		t.Fatalf(
			"error = %v, want selector failure",
			err,
		)
	}
	if len(output.Steps) != 1 {
		t.Fatalf(
			"steps = %d, want 1",
			len(output.Steps),
		)
	}
	if len(output.AcceptedRawDocuments) != 0 {
		t.Fatalf(
			"accepted raw documents = %d, want 0",
			len(output.AcceptedRawDocuments),
		)
	}
}

func TestAcquisitionPlanRunnerExecutesRemainingStepsAfterStructuralError(
	t *testing.T,
) {
	publishedAt := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)
	firstCalls := 0
	secondCalls := 0

	output, err := (AcquisitionPlanRunner{}).Run(
		context.Background(),
		ResearchTask{
			ID: "task-structural-error",
		},
		AcquisitionPlan{
			Steps: []AcquisitionStep{
				{
					ID:    "invalid-pit-step",
					Scope: AcquisitionScopeDirect,
					Task: ResearchTask{
						Requirements: ResearchRequirements{
							PITSafe: true,
						},
					},
					Retriever: countingRetriever{
						id:     "first_api",
						method: AcquisitionAPI,
						docs: []RawDocument{
							{
								ID:          "first",
								Title:       "first",
								Publisher:   "first",
								PublishedAt: &publishedAt,
							},
						},
						callCount: &firstCalls,
					},
				},
				{
					ID:    "valid-step",
					Scope: AcquisitionScopeDirect,
					Retriever: countingRetriever{
						id:     "second_api",
						method: AcquisitionAPI,
						docs: []RawDocument{
							{
								ID:          "second",
								Title:       "second",
								URL:         "https://example.com/second",
								Publisher:   "second",
								PublishedAt: &publishedAt,
							},
						},
						callCount: &secondCalls,
					},
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"as_of is required",
		) {
		t.Fatalf(
			"error = %v, want structural PIT error",
			err,
		)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf(
			"calls first=%d second=%d, want 1/1",
			firstCalls,
			secondCalls,
		)
	}
	if len(output.Steps) != 2 {
		t.Fatalf(
			"steps = %d, want 2",
			len(output.Steps),
		)
	}
	if output.Steps[0].ErrorMessage == "" {
		t.Fatal("structural step error should be preserved")
	}
}
