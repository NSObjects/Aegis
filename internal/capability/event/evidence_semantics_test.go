package event

import (
	"testing"
	"time"
)

func TestCanonicalizerPreservesExplicitEvidenceSemantics(t *testing.T) {
	publishedAt := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)

	result, err := (Canonicalizer{}).Canonicalize(
		ResearchTask{ID: "task-semantics"},
		[]RawDocument{
			{
				ID:           "raw-primary",
				Title:        "issuer disclosure",
				URL:          "https://example.com/disclosure/1",
				Publisher:    "distribution-channel",
				SourceType:   SourceOfficial,
				Authority:    AuthorityIssuer,
				EvidenceKind: EvidenceDisclosure,
				IsPrimary:    true,
				PublishedAt:  &publishedAt,
			},
			{
				ID:           "raw-duplicate",
				Title:        "same disclosure from duplicate acquisition",
				URL:          "https://example.com/disclosure/1",
				Publisher:    "mirror",
				SourceType:   SourceMainstreamMedia,
				Authority:    AuthorityMedia,
				EvidenceKind: EvidenceNewsReport,
				PublishedAt:  &publishedAt,
			},
		},
	)
	if err != nil {
		t.Fatalf("Canonicalize() error = %v", err)
	}
	if len(result.Documents) != 1 {
		t.Fatalf(
			"documents = %d, want 1",
			len(result.Documents),
		)
	}

	doc := result.Documents[0]
	if doc.Source.Authority != AuthorityIssuer {
		t.Fatalf(
			"authority = %q, want %q",
			doc.Source.Authority,
			AuthorityIssuer,
		)
	}
	if doc.Source.EvidenceKind != EvidenceDisclosure {
		t.Fatalf(
			"evidence kind = %q, want %q",
			doc.Source.EvidenceKind,
			EvidenceDisclosure,
		)
	}
	if doc.Source.SourceType != SourceOfficial {
		t.Fatalf(
			"source type = %q, want %q",
			doc.Source.SourceType,
			SourceOfficial,
		)
	}
}

func TestCanonicalizerDoesNotInferEvidenceSemantics(
	t *testing.T,
) {
	result, err := (Canonicalizer{}).Canonicalize(
		ResearchTask{ID: "task-no-inference"},
		[]RawDocument{
			{
				ID:         "raw-official",
				Title:      "official-looking document",
				URL:        "https://official.example/document",
				Publisher:  "official source",
				SourceType: SourceOfficial,
				IsPrimary:  true,
			},
		},
	)
	if err != nil {
		t.Fatalf("Canonicalize() error = %v", err)
	}
	if len(result.Documents) != 1 {
		t.Fatalf(
			"documents = %d, want 1",
			len(result.Documents),
		)
	}

	doc := result.Documents[0]
	if doc.Source.Authority != "" {
		t.Fatalf(
			"authority = %q, want empty without explicit retriever semantics",
			doc.Source.Authority,
		)
	}
	if doc.Source.EvidenceKind != "" {
		t.Fatalf(
			"evidence kind = %q, want empty without explicit retriever semantics",
			doc.Source.EvidenceKind,
		)
	}
}

func TestTaskEvaluatorTreatsSourceAuthorityAndKindIndependently(
	t *testing.T,
) {
	task := ResearchTask{
		ID: "task-evidence-contract",
		Requirements: ResearchRequirements{
			RequiredSourceTypes: []SourceType{
				SourceOfficial,
			},
			RequiredAuthorities: []EvidenceAuthority{
				AuthorityIssuer,
			},
			RequiredEvidenceKinds: []EvidenceKind{
				EvidenceDisclosure,
			},
			MinDocuments:         1,
			RequirePrimarySource: true,
		},
	}

	sourceOnly := CanonicalDocument{
		ID: "doc-source-only",
		Source: DocumentSource{
			Publisher:  "official distribution channel",
			URL:        "https://example.com/1",
			SourceType: SourceOfficial,
			IsPrimary:  true,
		},
	}

	result := (TaskEvaluator{}).Evaluate(
		task,
		[]CanonicalDocument{sourceOnly},
		nil,
	)
	if result.Status != TaskPartial {
		t.Fatalf(
			"status = %q, want %q; gaps=%v",
			result.Status,
			TaskPartial,
			result.Gaps,
		)
	}
	if hasGap(result.Gaps, GapMissingRequiredSourceType) {
		t.Fatalf(
			"source-type requirement should be satisfied: %v",
			result.Gaps,
		)
	}
	if !hasGap(
		result.Gaps,
		GapMissingRequiredAuthority,
	) {
		t.Fatalf(
			"gaps = %v, want authority gap",
			result.Gaps,
		)
	}
	if !hasGap(
		result.Gaps,
		GapMissingRequiredEvidenceKind,
	) {
		t.Fatalf(
			"gaps = %v, want evidence-kind gap",
			result.Gaps,
		)
	}

	complete := sourceOnly
	complete.Source.Authority = AuthorityIssuer
	complete.Source.EvidenceKind = EvidenceDisclosure

	result = (TaskEvaluator{}).Evaluate(
		task,
		[]CanonicalDocument{complete},
		nil,
	)
	if result.Status != TaskSatisfied {
		t.Fatalf(
			"status = %q, want %q; gaps=%v",
			result.Status,
			TaskSatisfied,
			result.Gaps,
		)
	}
}

func TestRealRetrieverEvidenceSemantics(t *testing.T) {
	t.Run("Cninfo issuer disclosure", func(t *testing.T) {
		publishedAt := time.Date(
			2026, 9, 29, 10, 0, 0, 0,
			cninfoShanghaiLocation,
		)

		document := cninfoRawDocument(
			ResearchTask{
				ID: "task-cninfo-semantics",
			},
			cninfoStock{
				Symbol:       "600519",
				InstrumentID: "600519.SH",
				Exchange:     "SSE",
				Name:         "贵州茅台",
			},
			map[string]any{
				"announcementId":    "semantic-1",
				"announcementTitle": "贵州茅台公告",
				"announcementTime":  publishedAt.UnixMilli(),
				"adjunctUrl":         "finalpage/semantic-1.PDF",
				"secName":            "贵州茅台",
			},
			publishedAt.Add(time.Hour),
		)
		if document == nil {
			t.Fatal("cninfoRawDocument() returned nil")
		}
		if document.Authority != AuthorityIssuer {
			t.Fatalf(
				"authority = %q, want %q",
				document.Authority,
				AuthorityIssuer,
			)
		}
		if document.EvidenceKind != EvidenceDisclosure {
			t.Fatalf(
				"evidence kind = %q, want %q",
				document.EvidenceKind,
				EvidenceDisclosure,
			)
		}
	})

	t.Run("CLS media news report", func(t *testing.T) {
		publishedAt := time.Date(
			2026, 9, 29, 10, 0, 0, 0,
			clsShanghaiLocation,
		)

		document := clsRawDocument(
			CLSNewsImportant,
			clsNewsItem{
				ID:      "semantic-2",
				Title:   "财联社快讯",
				Content: "新闻正文",
				Ctime:   publishedAt.Unix(),
			},
			publishedAt.Add(time.Hour),
		)
		if document == nil {
			t.Fatal("clsRawDocument() returned nil")
		}
		if document.Authority != AuthorityMedia {
			t.Fatalf(
				"authority = %q, want %q",
				document.Authority,
				AuthorityMedia,
			)
		}
		if document.EvidenceKind != EvidenceNewsReport {
			t.Fatalf(
				"evidence kind = %q, want %q",
				document.EvidenceKind,
				EvidenceNewsReport,
			)
		}
	})
}
