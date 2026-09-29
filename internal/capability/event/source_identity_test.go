package event

import (
	"context"
	"testing"
	"time"
)

func TestDomainSourceIdentityResolverKnownSources(
	t *testing.T,
) {
	resolver := DefaultSourceIdentityResolver()

	tests := []struct {
		name       string
		url        string
		sourceType SourceType
		authority  EvidenceAuthority
		kind       EvidenceKind
		primary    bool
		domain     string
		publisher  string
	}{
		{
			name:       "cninfo static subdomain",
			url:        "https://static.cninfo.com.cn/finalpage/a.pdf",
			sourceType: SourceOfficial,
			authority:  AuthorityIssuer,
			kind:       EvidenceDisclosure,
			primary:    true,
			domain:     "cninfo.com.cn",
			publisher:  "巨潮资讯网",
		},
		{
			name:       "sse subdomain",
			url:        "https://news.sse.com.cn/disclosure/a",
			sourceType: SourceOfficial,
			authority:  AuthorityExchange,
			kind:       EvidenceDisclosure,
			primary:    true,
			domain:     "sse.com.cn",
			publisher:  "上海证券交易所",
		},
		{
			name:       "csrc",
			url:        "https://www.csrc.gov.cn/csrc/c100028/common_list.shtml",
			sourceType: SourceOfficial,
			authority:  AuthorityRegulator,
			kind:       EvidenceDisclosure,
			primary:    true,
			domain:     "csrc.gov.cn",
			publisher:  "中国证券监督管理委员会",
		},
		{
			name:       "cls",
			url:        "https://www.cls.cn/detail/1",
			sourceType: SourceIndustryMedia,
			authority:  AuthorityMedia,
			kind:       EvidenceNewsReport,
			primary:    false,
			domain:     "cls.cn",
			publisher:  "财联社",
		},
		{
			name:       "reuters subdomain",
			url:        "https://www.reuters.com/world/example",
			sourceType: SourceMainstreamMedia,
			authority:  AuthorityMedia,
			kind:       EvidenceNewsReport,
			primary:    false,
			domain:     "reuters.com",
			publisher:  "Reuters",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity := resolver.Resolve(
				RawDocument{
					URL:          test.url,
					Publisher:    sourceIdentityHostname(test.url),
					SourceType:   SourceOther,
					EvidenceKind: EvidenceOther,
				},
			)
			if identity.SourceType != test.sourceType ||
				identity.Authority != test.authority ||
				identity.EvidenceKind != test.kind ||
				identity.IsPrimary != test.primary ||
				identity.CanonicalDomain != test.domain ||
				identity.Publisher != test.publisher {
				t.Fatalf(
					"identity = %+v",
					identity,
				)
			}
			if identity.ResolutionReason !=
				"domain_registry:"+test.domain {
				t.Fatalf(
					"resolution reason = %q",
					identity.ResolutionReason,
				)
			}
		})
	}
}

func TestDomainSourceIdentityResolverRejectsLookalikeDomains(
	t *testing.T,
) {
	resolver := DefaultSourceIdentityResolver()

	for _, rawURL := range []string{
		"https://evil-sse.com.cn/a",
		"https://sse.com.cn.evil.example/a",
		"https://evil.example/a?next=https://sse.com.cn",
	} {
		t.Run(rawURL, func(t *testing.T) {
			identity := resolver.Resolve(
				RawDocument{
					URL:          rawURL,
					SourceType:   SourceOther,
					EvidenceKind: EvidenceOther,
				},
			)
			if identity.SourceType != SourceOther {
				t.Fatalf(
					"lookalike source type = %q",
					identity.SourceType,
				)
			}
			if identity.Authority != "" {
				t.Fatalf(
					"lookalike authority = %q",
					identity.Authority,
				)
			}
			if identity.IsPrimary {
				t.Fatal(
					"lookalike domain became primary",
				)
			}
			if identity.ResolutionReason ==
				"domain_registry:sse.com.cn" {
				t.Fatalf(
					"lookalike matched registry: %+v",
					identity,
				)
			}
		})
	}
}

func TestDomainSourceIdentityResolverUnknownDomainStaysConservative(
	t *testing.T,
) {
	identity := DefaultSourceIdentityResolver().Resolve(
		RawDocument{
			URL:          "https://unknown.example/a",
			Publisher:    "unknown.example",
			SourceType:   SourceOther,
			EvidenceKind: EvidenceOther,
		},
	)

	if identity.SourceType != SourceOther ||
		identity.Authority != "" ||
		identity.EvidenceKind != EvidenceOther ||
		identity.IsPrimary {
		t.Fatalf(
			"unknown identity = %+v",
			identity,
		)
	}
	if identity.CanonicalDomain !=
		"unknown.example" {
		t.Fatalf(
			"canonical domain = %q",
			identity.CanonicalDomain,
		)
	}
	if identity.ResolutionReason !=
		"unclassified_domain:unknown.example" {
		t.Fatalf(
			"resolution reason = %q",
			identity.ResolutionReason,
		)
	}
}

func TestDomainSourceIdentityResolverDoesNotDowngradeSpecificProviderSemantics(
	t *testing.T,
) {
	identity := DefaultSourceIdentityResolver().Resolve(
		RawDocument{
			URL:          "https://www.reuters.com/example",
			Publisher:    "provider-declared",
			SourceType:   SourceResearch,
			Authority:    AuthorityResearch,
			EvidenceKind: EvidenceResearchAnalysis,
			IsPrimary:    true,
		},
	)

	if identity.Publisher != "provider-declared" ||
		identity.SourceType != SourceResearch ||
		identity.Authority != AuthorityResearch ||
		identity.EvidenceKind != EvidenceResearchAnalysis ||
		!identity.IsPrimary {
		t.Fatalf(
			"specific provider semantics were overwritten: %+v",
			identity,
		)
	}
	if identity.CanonicalDomain != "reuters.com" {
		t.Fatalf(
			"canonical domain = %q",
			identity.CanonicalDomain,
		)
	}
}

type sourceIdentityTestRetriever struct {
	document RawDocument
}

func (sourceIdentityTestRetriever) ID() string {
	return "source_identity_web_search"
}

func (sourceIdentityTestRetriever) Method() AcquisitionMethod {
	return AcquisitionWebSearch
}

func (r sourceIdentityTestRetriever) Retrieve(
	context.Context,
	ResearchTask,
) ([]RawDocument, error) {
	return []RawDocument{r.document}, nil
}

type sourceIdentitySnapshotSpy struct {
	saved []RawDocument
}

func (s *sourceIdentitySnapshotSpy) Save(
	_ context.Context,
	documents []RawDocument,
) error {
	s.saved = append(
		s.saved,
		documents...,
	)
	return nil
}

func (*sourceIdentitySnapshotSpy) Query(
	context.Context,
	EvidenceSnapshotQuery,
) ([]RawDocument, error) {
	return nil, nil
}

func TestTaskRunnerResolvesSourceIdentityBeforeSnapshotPersistence(
	t *testing.T,
) {
	now := time.Date(
		2026, 9, 30, 1, 0, 0, 0, time.UTC,
	)
	snapshot := &sourceIdentitySnapshotSpy{}

	output, err := (TaskRunner{
		Now: func() time.Time {
			return now
		},
		SnapshotStore: snapshot,
	}).Run(
		context.Background(),
		ResearchTask{
			ID: "source-identity-task",
		},
		sourceIdentityTestRetriever{
			document: RawDocument{
				ID:          "web-result",
				URL:         "https://news.sse.com.cn/disclosure/1",
				Publisher:   "news.sse.com.cn",
				SourceType:  SourceOther,
				EvidenceKind: EvidenceOther,
				AvailableAt: cloneTime(&now),
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"TaskRunner.Run() error = %v",
			err,
		)
	}
	if output.Attempt.Method != AcquisitionWebSearch {
		t.Fatalf(
			"acquisition method = %q",
			output.Attempt.Method,
		)
	}
	if len(snapshot.saved) != 1 {
		t.Fatalf(
			"saved snapshots = %d, want 1",
			len(snapshot.saved),
		)
	}

	saved := snapshot.saved[0]
	if saved.SourceType != SourceOfficial ||
		saved.Authority != AuthorityExchange ||
		saved.EvidenceKind != EvidenceDisclosure ||
		!saved.IsPrimary ||
		saved.CanonicalDomain != "sse.com.cn" {
		t.Fatalf(
			"saved identity = %+v",
			saved,
		)
	}
	if saved.ResolutionReason !=
		"domain_registry:sse.com.cn" {
		t.Fatalf(
			"saved resolution reason = %q",
			saved.ResolutionReason,
		)
	}

	if len(output.Canonicalization.Documents) != 1 {
		t.Fatalf(
			"canonical documents = %d, want 1",
			len(output.Canonicalization.Documents),
		)
	}
	source := output.Canonicalization.Documents[0].Source
	if source.CanonicalDomain != "sse.com.cn" ||
		source.ResolutionReason !=
			"domain_registry:sse.com.cn" {
		t.Fatalf(
			"canonical source = %+v",
			source,
		)
	}
}
