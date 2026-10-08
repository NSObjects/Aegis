package event

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type targetResolverFunc func(
	context.Context,
	ResearchTask,
) (EvidenceTarget, error)

func (f targetResolverFunc) Resolve(
	ctx context.Context,
	task ResearchTask,
) (EvidenceTarget, error) {
	return f(ctx, task)
}

type semanticMatcherFunc func(
	context.Context,
	EvidenceTarget,
	[]RawDocument,
) ([]SemanticEvidenceMatch, error)

func (f semanticMatcherFunc) Match(
	ctx context.Context,
	target EvidenceTarget,
	documents []RawDocument,
) ([]SemanticEvidenceMatch, error) {
	return f(ctx, target, documents)
}

func TestHybridEvidenceSelectorDeterministicAndSemantic(
	t *testing.T,
) {
	semanticCalls := 0
	selector := &HybridEvidenceSelector{
		TargetResolver: targetResolverFunc(
			func(
				context.Context,
				ResearchTask,
			) (EvidenceTarget, error) {
				return EvidenceTarget{
					Symbol:  "600519.SH",
					Name:    "贵州茅台",
					Aliases: []string{"茅台"},
				}, nil
			},
		),
		SemanticMatcher: semanticMatcherFunc(
			func(
				_ context.Context,
				target EvidenceTarget,
				documents []RawDocument,
			) ([]SemanticEvidenceMatch, error) {
				semanticCalls++
				if target.Name != "贵州茅台" {
					t.Fatalf("target = %+v", target)
				}
				if len(documents) != 2 {
					t.Fatalf(
						"semantic documents = %d, want 2",
						len(documents),
					)
				}
				return []SemanticEvidenceMatch{
					{
						RawDocumentID: "semantic-high",
						Relevant:      true,
						Confidence:    0.92,
						Reason:        "direct supply contract affects target",
					},
					{
						RawDocumentID: "semantic-low",
						Relevant:      true,
						Confidence:    0.60,
						Reason:        "weak thematic relation",
					},
				}, nil
			},
		),
		MinConfidence: 0.80,
	}

	decisions, err := selector.Select(
		context.Background(),
		ResearchTask{
			ID:     "task-selector",
			Symbol: "600519.SH",
		},
		[]RawDocument{
			{
				ID:      "metadata",
				Title:   "unrelated title",
				Content: "unrelated body",
				RawMetadata: map[string]any{
					"instrument_id": "600519.SH",
				},
			},
			{
				ID:      "name",
				Title:   "贵州茅台发布公告",
				Content: "content",
			},
			{
				ID:      "alias",
				Title:   "茅台渠道调整",
				Content: "content",
			},
			{
				ID:      "semantic-high",
				Title:   "供应商签署重大合同",
				Content: "合同对象未在字符串中明确写出股票简称",
			},
			{
				ID:      "semantic-low",
				Title:   "白酒行业景气变化",
				Content: "仅存在弱关系",
			},
		},
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if semanticCalls != 1 {
		t.Fatalf(
			"semantic matcher calls = %d, want 1",
			semanticCalls,
		)
	}
	if len(decisions) != 5 {
		t.Fatalf(
			"decisions = %d, want 5",
			len(decisions),
		)
	}

	byID := make(map[string]EvidenceSelectionDecision)
	for _, decision := range decisions {
		byID[decision.RawDocumentID] = decision
	}

	for _, id := range []string{
		"metadata",
		"name",
		"alias",
	} {
		decision := byID[id]
		if !decision.Selected ||
			decision.Method !=
				SelectionDeterministic ||
			decision.Confidence != 1 {
			t.Fatalf(
				"deterministic decision %s = %+v",
				id,
				decision,
			)
		}
	}

	high := byID["semantic-high"]
	if !high.Selected ||
		high.Method != SelectionSemantic ||
		high.Confidence != 0.92 {
		t.Fatalf(
			"semantic-high = %+v",
			high,
		)
	}

	low := byID["semantic-low"]
	if low.Selected ||
		low.Method != SelectionSemantic ||
		low.Confidence != 0.60 ||
		!strings.Contains(
			low.Reason,
			"below selection threshold",
		) {
		t.Fatalf(
			"semantic-low = %+v",
			low,
		)
	}
}

func TestHybridEvidenceSelectorDoesNotRejectUnmatchedTextWithoutSemanticReview(
	t *testing.T,
) {
	called := false
	selector := &HybridEvidenceSelector{
		TargetResolver: targetResolverFunc(
			func(
				context.Context,
				ResearchTask,
			) (EvidenceTarget, error) {
				return EvidenceTarget{
					Symbol: "600519.SH",
					Name:   "贵州茅台",
				}, nil
			},
		),
		SemanticMatcher: semanticMatcherFunc(
			func(
				_ context.Context,
				_ EvidenceTarget,
				documents []RawDocument,
			) ([]SemanticEvidenceMatch, error) {
				called = true
				return []SemanticEvidenceMatch{
					{
						RawDocumentID: documents[0].ID,
						Relevant:      false,
						Confidence:    0.95,
						Reason:        "document is about another company",
					},
				}, nil
			},
		),
	}

	decisions, err := selector.Select(
		context.Background(),
		ResearchTask{
			ID:     "task-no-string-match",
			Symbol: "600519.SH",
		},
		[]RawDocument{
			{
				ID:      "candidate",
				Title:   "某公司公告",
				Content: "没有目标名称字符串",
			},
		},
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if !called {
		t.Fatal("unmatched text must be delegated to semantic matcher")
	}
	if len(decisions) != 1 ||
		decisions[0].Selected {
		t.Fatalf(
			"decisions = %+v, want semantic rejection",
			decisions,
		)
	}
}

func TestHybridEvidenceSelectorValidation(
	t *testing.T,
) {
	baseResolver := targetResolverFunc(
		func(
			context.Context,
			ResearchTask,
		) (EvidenceTarget, error) {
			return EvidenceTarget{
				Symbol: "600519.SH",
				Name:   "贵州茅台",
			}, nil
		},
	)
	document := RawDocument{
		ID:      "candidate",
		Title:   "no deterministic match",
		Content: "semantic review needed",
	}

	tests := []struct {
		name    string
		selector *HybridEvidenceSelector
		want    string
	}{
		{
			name: "missing resolver",
			selector: &HybridEvidenceSelector{},
			want: "target resolver is required",
		},
		{
			name: "missing semantic matcher",
			selector: &HybridEvidenceSelector{
				TargetResolver: baseResolver,
			},
			want: "semantic evidence matcher is required",
		},
		{
			name: "unknown semantic id",
			selector: &HybridEvidenceSelector{
				TargetResolver: baseResolver,
				SemanticMatcher: semanticMatcherFunc(
					func(
						context.Context,
						EvidenceTarget,
						[]RawDocument,
					) ([]SemanticEvidenceMatch, error) {
						return []SemanticEvidenceMatch{
							{
								RawDocumentID: "unknown",
								Confidence:    1,
							},
						}, nil
					},
				),
			},
			want: "unknown raw document",
		},
		{
			name: "duplicate semantic id",
			selector: &HybridEvidenceSelector{
				TargetResolver: baseResolver,
				SemanticMatcher: semanticMatcherFunc(
					func(
						context.Context,
						EvidenceTarget,
						[]RawDocument,
					) ([]SemanticEvidenceMatch, error) {
						return []SemanticEvidenceMatch{
							{
								RawDocumentID: "candidate",
								Confidence:    1,
								Reason: "test classification",
							},
							{
								RawDocumentID: "candidate",
								Confidence:    1,
								Reason: "test classification",
							},
						}, nil
					},
				),
			},
			want: "duplicated",
		},
		{
			name: "invalid confidence",
			selector: &HybridEvidenceSelector{
				TargetResolver: baseResolver,
				SemanticMatcher: semanticMatcherFunc(
					func(
						context.Context,
						EvidenceTarget,
						[]RawDocument,
					) ([]SemanticEvidenceMatch, error) {
						return []SemanticEvidenceMatch{
							{
								RawDocumentID: "candidate",
								Confidence:    1.2,
							},
						}, nil
					},
				),
			},
			want: "must be between 0 and 1",
		},
		{
			name: "incomplete semantic coverage",
			selector: &HybridEvidenceSelector{
				TargetResolver: baseResolver,
				SemanticMatcher: semanticMatcherFunc(func(
					context.Context, EvidenceTarget, []RawDocument,
				) ([]SemanticEvidenceMatch, error) {
					return nil, nil
				}),
			},
			want: "research incomplete",
		},
		{
			name: "semantic failure",
			selector: &HybridEvidenceSelector{
				TargetResolver: baseResolver,
				SemanticMatcher: semanticMatcherFunc(
					func(
						context.Context,
						EvidenceTarget,
						[]RawDocument,
					) ([]SemanticEvidenceMatch, error) {
						return nil, errors.New(
							"model unavailable",
						)
					},
				),
			},
			want: "model unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.selector.Select(
				context.Background(),
				ResearchTask{
					ID:     "task-validation",
					Symbol: "600519.SH",
				},
				[]RawDocument{document},
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

func TestHybridEvidenceSelectorRejectsTargetSymbolMismatch(
	t *testing.T,
) {
	selector := &HybridEvidenceSelector{
		TargetResolver: targetResolverFunc(
			func(
				context.Context,
				ResearchTask,
			) (EvidenceTarget, error) {
				return EvidenceTarget{
					Symbol: "000001.SZ",
					Name:   "other",
				}, nil
			},
		),
	}

	_, err := selector.Select(
		context.Background(),
		ResearchTask{
			ID:     "task-symbol-mismatch",
			Symbol: "600519.SH",
		},
		nil,
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"does not match task symbol",
		) {
		t.Fatalf(
			"error = %v, want symbol mismatch",
			err,
		)
	}
}

func TestMapEvidenceTargetResolver(t *testing.T) {
	resolver := MapEvidenceTargetResolver{
		"600519.SH": {
			Name:    "贵州茅台",
			Aliases: []string{"茅台"},
		},
	}

	target, err := resolver.Resolve(
		context.Background(),
		ResearchTask{
			Symbol: "600519.sh",
		},
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if target.Symbol != "600519.SH" ||
		target.Name != "贵州茅台" {
		t.Fatalf("target = %+v", target)
	}

	_, err = resolver.Resolve(
		context.Background(),
		ResearchTask{
			Symbol: "000001.SZ",
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"is not configured",
		) {
		t.Fatalf(
			"error = %v, want missing-target error",
			err,
		)
	}
}

func TestHybridEvidenceSelectorRejectsIncompleteSemanticCoverage(t *testing.T) {
	selector := &HybridEvidenceSelector{
		TargetResolver: MapEvidenceTargetResolver{
			"600519.SH": {Name: "贵州茅台"},
		},
		SemanticMatcher: semanticMatcherFunc(func(
			_ context.Context, _ EvidenceTarget, documents []RawDocument,
		) ([]SemanticEvidenceMatch, error) {
			if len(documents) != 2 {
				t.Fatalf("documents = %d, want 2", len(documents))
			}
			return []SemanticEvidenceMatch{{
				RawDocumentID: "candidate-1", Relevant: true,
				Confidence: .93, Reason: "issuer linkage",
			}}, nil
		}),
	}
	decisions, err := selector.Select(context.Background(),
		ResearchTask{Symbol: "600519.SH"},
		[]RawDocument{
			{ID: "candidate-1", Title: "supplier news"},
			{ID: "cls_news_2498111", Title: "market news"},
		})
	if err == nil || !strings.Contains(err.Error(), "research incomplete") ||
		!strings.Contains(err.Error(), "cls_news_2498111") {
		t.Fatalf("error = %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("incomplete selection cannot appear successful: %+v", decisions)
	}
}

func TestHybridEvidenceSelectorRejectsAllMissingSemanticCoverage(t *testing.T) {
	_, err := validateSemanticMatches(
		[]RawDocument{{ID: "candidate-1"}, {ID: "candidate-2"}},
		nil, .8,
	)
	if err == nil || !strings.Contains(err.Error(), "research incomplete") {
		t.Fatalf("error = %v", err)
	}
}

func TestHybridEvidenceSelectorPartialSemanticMatchesStillRejectsInvalidIDs(t *testing.T) {
	for _, tc := range []struct{
		name string
		matches []SemanticEvidenceMatch
		want string
	}{
		{"unknown", []SemanticEvidenceMatch{{RawDocumentID:"unknown", Confidence:.7}}, "unknown raw document"},
		{"duplicate", []SemanticEvidenceMatch{{RawDocumentID:"one", Confidence:.7, Reason:"test classification"}, {RawDocumentID:"one", Confidence:.7, Reason:"test classification"}}, "duplicated"},
		{"bad confidence", []SemanticEvidenceMatch{{RawDocumentID:"one", Confidence:1.5}}, "between 0 and 1"},
	} {
		t.Run(tc.name,func(t *testing.T){
			_,err:=validateSemanticMatches([]RawDocument{{ID:"one"}, {ID:"missing"}},tc.matches,.8)
			if err==nil || !strings.Contains(err.Error(),tc.want) {
				t.Fatalf("error=%v, want %q",err,tc.want)
			}
		})
	}
}
