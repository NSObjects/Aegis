package event

import (
	"reflect"
	"strings"
	"testing"
)

func TestEvidenceSupportCollapsesReprintAndKeepsIndependentReport(
	t *testing.T,
) {
	original := supportTestDocument(
		"doc-a",
		"issuer.example",
		AuthorityIssuer,
		EvidenceDisclosure,
		true,
		"same filing body",
	)
	reprint := supportTestDocument(
		"doc-b",
		"media.example",
		AuthorityMedia,
		EvidenceNewsReport,
		false,
		"same filing body",
	)
	independent := supportTestDocument(
		"doc-c",
		"other.example",
		AuthorityMedia,
		EvidenceNewsReport,
		false,
		"independent reporting body",
	)

	documents := []CanonicalDocument{
		original,
		reprint,
		independent,
	}
	corroboration := (EvidenceCorroborator{}).
		Corroborate(documents)

	support, err := (EvidenceSupportAnalyzer{}).Analyze(
		[]string{"doc-c", "doc-b", "doc-a"},
		documents,
		corroboration,
	)
	if err != nil {
		t.Fatal(err)
	}

	if support.DocumentCount != 3 {
		t.Fatalf(
			"document count = %d, want 3",
			support.DocumentCount,
		)
	}
	if support.IndependentEvidenceCount != 2 {
		t.Fatalf(
			"independent evidence = %d, want 2",
			support.IndependentEvidenceCount,
		)
	}
	if support.IndependentSourceCount != 2 {
		t.Fatalf(
			"independent sources = %d, want 2",
			support.IndependentSourceCount,
		)
	}
	if support.CollapsedReprintCount != 1 {
		t.Fatalf(
			"collapsed reprints = %d, want 1",
			support.CollapsedReprintCount,
		)
	}
	if !support.PrimarySourcePresent {
		t.Fatal("primary source should be present")
	}
	if support.SupportClass !=
		EvidenceSupportPrimaryCorroborated {
		t.Fatalf(
			"support class = %q, want %q",
			support.SupportClass,
			EvidenceSupportPrimaryCorroborated,
		)
	}
}

func TestEvidenceSupportDoesNotInventUncitedClusterOrigin(
	t *testing.T,
) {
	original := supportTestDocument(
		"doc-a",
		"issuer.example",
		AuthorityIssuer,
		EvidenceDisclosure,
		true,
		"same filing body",
	)
	reprint := supportTestDocument(
		"doc-b",
		"media.example",
		AuthorityMedia,
		EvidenceNewsReport,
		false,
		"same filing body",
	)
	documents := []CanonicalDocument{
		original,
		reprint,
	}

	support, err := (EvidenceSupportAnalyzer{}).Analyze(
		[]string{"doc-b"},
		documents,
		(EvidenceCorroborator{}).
			Corroborate(documents),
	)
	if err != nil {
		t.Fatal(err)
	}

	if support.IndependentSourceCount != 1 {
		t.Fatalf(
			"independent sources = %d, want 1",
			support.IndependentSourceCount,
		)
	}
	if support.PrimarySourcePresent {
		t.Fatal(
			"uncited primary cluster member must not become event primary evidence",
		)
	}
	if support.SupportClass !=
		EvidenceSupportSingleSource {
		t.Fatalf(
			"support class = %q, want %q",
			support.SupportClass,
			EvidenceSupportSingleSource,
		)
	}
}

func TestEvidenceSupportSamePublisherIndependentEvidenceUnits(
	t *testing.T,
) {
	first := supportTestDocument(
		"doc-a",
		"media.example",
		AuthorityMedia,
		EvidenceNewsReport,
		false,
		"first independent article",
	)
	second := supportTestDocument(
		"doc-b",
		"media.example",
		AuthorityMedia,
		EvidenceNewsReport,
		false,
		"second independent article",
	)
	documents := []CanonicalDocument{
		first,
		second,
	}

	support, err := (EvidenceSupportAnalyzer{}).Analyze(
		[]string{"doc-a", "doc-b"},
		documents,
		(EvidenceCorroborator{}).
			Corroborate(documents),
	)
	if err != nil {
		t.Fatal(err)
	}

	if support.IndependentEvidenceCount != 2 {
		t.Fatalf(
			"independent evidence = %d, want 2",
			support.IndependentEvidenceCount,
		)
	}
	if support.IndependentSourceCount != 1 {
		t.Fatalf(
			"independent sources = %d, want 1",
			support.IndependentSourceCount,
		)
	}
	if support.SupportClass !=
		EvidenceSupportSingleSource {
		t.Fatalf(
			"support class = %q, want %q",
			support.SupportClass,
			EvidenceSupportSingleSource,
		)
	}
}

func TestEvidenceSupportUnknownSourceDoesNotInflateSources(
	t *testing.T,
) {
	document := CanonicalDocument{
		ID:          "doc-unknown",
		Content:     "unknown source content",
		ContentHash: contentHash("unknown source content"),
	}
	documents := []CanonicalDocument{document}

	support, err := (EvidenceSupportAnalyzer{}).Analyze(
		[]string{"doc-unknown"},
		documents,
		(EvidenceCorroborator{}).
			Corroborate(documents),
	)
	if err != nil {
		t.Fatal(err)
	}

	if support.IndependentEvidenceCount != 1 {
		t.Fatalf(
			"independent evidence = %d, want 1",
			support.IndependentEvidenceCount,
		)
	}
	if support.IndependentSourceCount != 0 {
		t.Fatalf(
			"independent sources = %d, want 0",
			support.IndependentSourceCount,
		)
	}
	if support.UnknownSourceCount != 1 {
		t.Fatalf(
			"unknown sources = %d, want 1",
			support.UnknownSourceCount,
		)
	}
	if support.SupportClass != EvidenceSupportUnknown {
		t.Fatalf(
			"support class = %q, want %q",
			support.SupportClass,
			EvidenceSupportUnknown,
		)
	}
}

func TestEvidenceSupportPrimaryOnlyIsDistinct(
	t *testing.T,
) {
	document := supportTestDocument(
		"doc-official",
		"issuer.example",
		AuthorityIssuer,
		EvidenceDisclosure,
		true,
		"official filing",
	)
	documents := []CanonicalDocument{document}

	support, err := (EvidenceSupportAnalyzer{}).Analyze(
		[]string{"doc-official"},
		documents,
		(EvidenceCorroborator{}).
			Corroborate(documents),
	)
	if err != nil {
		t.Fatal(err)
	}

	if support.SupportClass !=
		EvidenceSupportPrimaryOnly {
		t.Fatalf(
			"support class = %q, want %q",
			support.SupportClass,
			EvidenceSupportPrimaryOnly,
		)
	}
}

func TestEvidenceSupportDeterministicAcrossOrdering(
	t *testing.T,
) {
	base := strings.Repeat(
		"公司公告显示经营保持稳定渠道库存合理销售计划正常推进投资者应关注相关风险",
		5,
	)
	first := supportTestDocument(
		"doc-a",
		"issuer.example",
		AuthorityIssuer,
		EvidenceDisclosure,
		true,
		base,
	)
	second := supportTestDocument(
		"doc-b",
		"media.example",
		AuthorityMedia,
		EvidenceNewsReport,
		false,
		"媒体导语"+base,
	)
	third := supportTestDocument(
		"doc-c",
		"other.example",
		AuthorityResearch,
		EvidenceResearchAnalysis,
		false,
		"independent analysis",
	)

	leftDocuments := []CanonicalDocument{
		first,
		second,
		third,
	}
	rightDocuments := []CanonicalDocument{
		third,
		second,
		first,
	}

	left, err := (EvidenceSupportAnalyzer{}).Analyze(
		[]string{"doc-c", "doc-b", "doc-a"},
		leftDocuments,
		(EvidenceCorroborator{}).
			Corroborate(leftDocuments),
	)
	if err != nil {
		t.Fatal(err)
	}
	right, err := (EvidenceSupportAnalyzer{}).Analyze(
		[]string{"doc-a", "doc-b", "doc-c"},
		rightDocuments,
		(EvidenceCorroborator{}).
			Corroborate(rightDocuments),
	)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(left, right) {
		t.Fatalf(
			"support differs:\nleft=%+v\nright=%+v",
			left,
			right,
		)
	}
}

func supportTestDocument(
	id string,
	domain string,
	authority EvidenceAuthority,
	kind EvidenceKind,
	primary bool,
	content string,
) CanonicalDocument {
	return CanonicalDocument{
		ID:      id,
		Content: content,
		ContentHash: contentHash(
			strings.TrimSpace(content),
		),
		Source: DocumentSource{
			Publisher:       domain,
			CanonicalDomain: domain,
			Authority:       authority,
			EvidenceKind:    kind,
			IsPrimary:       primary,
		},
	}
}
