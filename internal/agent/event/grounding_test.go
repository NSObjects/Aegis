package event

import (
	capevent "Aegis/internal/capability/event"
	"reflect"
	"strings"
	"testing"
)

func TestResolveEvidenceGroundingsDerivesUTF8ByteSpan(
	t *testing.T,
) {
	content := "前言：贵州茅台发布年度报告，营业收入同比增长。结尾"
	documents := map[string]capevent.CanonicalDocument{
		"doc-a": {
			ID:      "doc-a",
			Content: content,
		},
	}

	groundings, err := ResolveEvidenceGroundings(
		[]EvidenceQuote{
			{
				DocumentID: "doc-a",
				Quote: "贵州茅台发布年度报告，营业收入同比增长。",
			},
		},
		documents,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(groundings) != 1 {
		t.Fatalf(
			"groundings = %d, want 1",
			len(groundings),
		)
	}
	got := groundings[0]
	if content[got.StartByte:got.EndByte] !=
		got.Quote {
		t.Fatalf(
			"span = %q, quote = %q",
			content[got.StartByte:got.EndByte],
			got.Quote,
		)
	}
	if got.StartByte != len("前言：") {
		t.Fatalf(
			"start byte = %d, want %d",
			got.StartByte,
			len("前言："),
		)
	}
}

func TestResolveEvidenceGroundingsRejectsWrongDocument(
	t *testing.T,
) {
	documents := map[string]capevent.CanonicalDocument{
		"doc-a": {
			ID:      "doc-a",
			Content: "公司签署重大合同。",
		},
		"doc-b": {
			ID:      "doc-b",
			Content: "公司发布例行公告。",
		},
	}

	_, err := ResolveEvidenceGroundings(
		[]EvidenceQuote{
			{
				DocumentID: "doc-b",
				Quote:      "公司签署重大合同。",
			},
		},
		documents,
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"not an exact substring",
		) {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveEvidenceGroundingsRejectsAmbiguousQuote(
	t *testing.T,
) {
	documents := map[string]capevent.CanonicalDocument{
		"doc-a": {
			ID: "doc-a",
			Content: "公司完成回购。其他内容。公司完成回购。",
		},
	}

	_, err := ResolveEvidenceGroundings(
		[]EvidenceQuote{
			{
				DocumentID: "doc-a",
				Quote:      "公司完成回购。",
			},
		},
		documents,
	)
	if err == nil ||
		!strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveEvidenceGroundingsRejectsEmptyQuote(
	t *testing.T,
) {
	_, err := ResolveEvidenceGroundings(
		[]EvidenceQuote{
			{
				DocumentID: "doc-a",
				Quote:      "   ",
			},
		},
		map[string]capevent.CanonicalDocument{
			"doc-a": {
				ID:      "doc-a",
				Content: "content",
			},
		},
	)
	if err == nil ||
		!strings.Contains(err.Error(), "quote is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveEvidenceGroundingsDeduplicatesAndSorts(
	t *testing.T,
) {
	documents := map[string]capevent.CanonicalDocument{
		"doc-a": {
			ID:      "doc-a",
			Content: "alpha beta gamma",
		},
		"doc-b": {
			ID:      "doc-b",
			Content: "delta epsilon",
		},
	}
	quotes := []EvidenceQuote{
		{DocumentID: "doc-b", Quote: "epsilon"},
		{DocumentID: "doc-a", Quote: "beta"},
		{DocumentID: "doc-a", Quote: "beta"},
	}

	left, err := ResolveEvidenceGroundings(
		quotes,
		documents,
	)
	if err != nil {
		t.Fatal(err)
	}
	right, err := ResolveEvidenceGroundings(
		[]EvidenceQuote{
			quotes[1],
			quotes[0],
			quotes[2],
		},
		documents,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf(
			"groundings differ:\nleft=%+v\nright=%+v",
			left,
			right,
		)
	}
	if len(left) != 2 {
		t.Fatalf(
			"groundings = %d, want 2",
			len(left),
		)
	}
	if !reflect.DeepEqual(
		groundingDocumentIDs(left),
		[]string{"doc-a", "doc-b"},
	) {
		t.Fatalf(
			"document ids = %v",
			groundingDocumentIDs(left),
		)
	}
}
