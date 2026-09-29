package event

import (
	"net/url"
	"strings"
)

type SourceIdentity struct {
	Publisher        string
	SourceType       SourceType
	Authority        EvidenceAuthority
	EvidenceKind     EvidenceKind
	IsPrimary        bool
	CanonicalDomain  string
	ResolutionReason string
}

type SourceIdentityResolver interface {
	Resolve(document RawDocument) SourceIdentity
}

type DomainSourceIdentityResolver struct {
	Rules []SourceIdentityDomainRule
}

type SourceIdentityDomainRule struct {
	Domain       string
	Publisher    string
	SourceType   SourceType
	Authority    EvidenceAuthority
	EvidenceKind EvidenceKind
	IsPrimary    bool
}

var defaultSourceIdentityRules = []SourceIdentityDomainRule{
	{
		Domain:       "cninfo.com.cn",
		Publisher:    "巨潮资讯网",
		SourceType:   SourceOfficial,
		Authority:    AuthorityIssuer,
		EvidenceKind: EvidenceDisclosure,
		IsPrimary:    true,
	},
	{
		Domain:       "sse.com.cn",
		Publisher:    "上海证券交易所",
		SourceType:   SourceOfficial,
		Authority:    AuthorityExchange,
		EvidenceKind: EvidenceDisclosure,
		IsPrimary:    true,
	},
	{
		Domain:       "szse.cn",
		Publisher:    "深圳证券交易所",
		SourceType:   SourceOfficial,
		Authority:    AuthorityExchange,
		EvidenceKind: EvidenceDisclosure,
		IsPrimary:    true,
	},
	{
		Domain:       "bse.cn",
		Publisher:    "北京证券交易所",
		SourceType:   SourceOfficial,
		Authority:    AuthorityExchange,
		EvidenceKind: EvidenceDisclosure,
		IsPrimary:    true,
	},
	{
		Domain:       "csrc.gov.cn",
		Publisher:    "中国证券监督管理委员会",
		SourceType:   SourceOfficial,
		Authority:    AuthorityRegulator,
		EvidenceKind: EvidenceDisclosure,
		IsPrimary:    true,
	},
	{
		Domain:       "cls.cn",
		Publisher:    "财联社",
		SourceType:   SourceIndustryMedia,
		Authority:    AuthorityMedia,
		EvidenceKind: EvidenceNewsReport,
		IsPrimary:    false,
	},
	{
		Domain:       "xinhuanet.com",
		Publisher:    "新华社",
		SourceType:   SourceMainstreamMedia,
		Authority:    AuthorityMedia,
		EvidenceKind: EvidenceNewsReport,
		IsPrimary:    false,
	},
	{
		Domain:       "xinhua.net",
		Publisher:    "新华社",
		SourceType:   SourceMainstreamMedia,
		Authority:    AuthorityMedia,
		EvidenceKind: EvidenceNewsReport,
		IsPrimary:    false,
	},
	{
		Domain:       "reuters.com",
		Publisher:    "Reuters",
		SourceType:   SourceMainstreamMedia,
		Authority:    AuthorityMedia,
		EvidenceKind: EvidenceNewsReport,
		IsPrimary:    false,
	},
}

func DefaultSourceIdentityResolver() SourceIdentityResolver {
	rules := make(
		[]SourceIdentityDomainRule,
		len(defaultSourceIdentityRules),
	)
	copy(rules, defaultSourceIdentityRules)
	return DomainSourceIdentityResolver{
		Rules: rules,
	}
}

func (r DomainSourceIdentityResolver) Resolve(
	document RawDocument,
) SourceIdentity {
	host := sourceIdentityHostname(document.URL)
	identity := sourceIdentityFromDocument(
		document,
		host,
	)

	if strings.TrimSpace(
		document.CanonicalDomain,
	) != "" &&
		strings.TrimSpace(
			document.ResolutionReason,
		) != "" {
		identity.CanonicalDomain = strings.ToLower(
			strings.TrimSpace(
				document.CanonicalDomain,
			),
		)
		return identity
	}

	if host == "" {
		identity.ResolutionReason =
			"url_hostname_unavailable"
		return identity
	}

	rule, matched := matchSourceIdentityDomainRule(
		host,
		r.Rules,
	)
	if !matched {
		identity.ResolutionReason =
			"unclassified_domain:" + host
		return identity
	}

	wasGeneric := sourceIdentityIsGeneric(
		document,
	)

	if wasGeneric {
		identity.Publisher = rule.Publisher
		identity.SourceType = rule.SourceType
		identity.Authority = rule.Authority
		identity.EvidenceKind = rule.EvidenceKind
		identity.IsPrimary = rule.IsPrimary
	}

	identity.CanonicalDomain = rule.Domain
	if wasGeneric {
		identity.ResolutionReason =
			"domain_registry:" + rule.Domain
	} else {
		identity.ResolutionReason =
			"domain_registry:" + rule.Domain +
				";provider_semantics_preserved"
	}
	return identity
}

func ApplySourceIdentity(
	document RawDocument,
	identity SourceIdentity,
) RawDocument {
	document.Publisher = strings.TrimSpace(
		identity.Publisher,
	)
	document.SourceType = identity.SourceType
	document.Authority = identity.Authority
	document.EvidenceKind = identity.EvidenceKind
	document.IsPrimary = identity.IsPrimary
	document.CanonicalDomain = strings.ToLower(
		strings.TrimSpace(
			identity.CanonicalDomain,
		),
	)
	document.ResolutionReason = strings.TrimSpace(
		identity.ResolutionReason,
	)
	return document
}

func sourceIdentityFromDocument(
	document RawDocument,
	host string,
) SourceIdentity {
	sourceType := document.SourceType
	if sourceType == "" {
		sourceType = SourceOther
	}
	evidenceKind := document.EvidenceKind
	if evidenceKind == "" {
		evidenceKind = EvidenceOther
	}

	publisher := strings.TrimSpace(
		document.Publisher,
	)
	if publisher == "" {
		publisher = host
	}

	return SourceIdentity{
		Publisher:        publisher,
		SourceType:       sourceType,
		Authority:        document.Authority,
		EvidenceKind:     evidenceKind,
		IsPrimary:        document.IsPrimary,
		CanonicalDomain:  host,
		ResolutionReason: strings.TrimSpace(document.ResolutionReason),
	}
}

func sourceIdentityIsGeneric(
	document RawDocument,
) bool {
	return (document.SourceType == "" ||
		document.SourceType == SourceOther) &&
		(document.Authority == "" ||
			document.Authority == AuthorityOther) &&
		(document.EvidenceKind == "" ||
			document.EvidenceKind == EvidenceOther)
}

func sourceIdentityHostname(
	rawURL string,
) string {
	parsed, err := url.Parse(
		strings.TrimSpace(rawURL),
	)
	if err != nil {
		return ""
	}
	host := strings.ToLower(
		strings.TrimSuffix(
			strings.TrimSpace(parsed.Hostname()),
			".",
		),
	)
	return host
}

func matchSourceIdentityDomainRule(
	host string,
	rules []SourceIdentityDomainRule,
) (SourceIdentityDomainRule, bool) {
	host = strings.ToLower(
		strings.TrimSuffix(
			strings.TrimSpace(host),
			".",
		),
	)

	var best SourceIdentityDomainRule
	bestLength := -1
	for _, rule := range rules {
		domain := strings.ToLower(
			strings.TrimSuffix(
				strings.TrimSpace(rule.Domain),
				".",
			),
		)
		if domain == "" {
			continue
		}
		if host != domain &&
			!strings.HasSuffix(
				host,
				"."+domain,
			) {
			continue
		}
		if len(domain) <= bestLength {
			continue
		}
		rule.Domain = domain
		best = rule
		bestLength = len(domain)
	}

	return best, bestLength >= 0
}
