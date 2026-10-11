package event

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	capevent "Aegis/internal/capability/event"
)

// CitedDocumentSource is selected from the actual, PIT-filtered canonical
// documents consumed by Event analysis. No document body, query parameters,
// API credentials, retriever raw metadata, or inferred links are persisted.
// An original source address can be absent if it is not safe to expose.
type CitedDocumentSource struct {
	DocumentID string `json:"document_id"`
	VersionID string `json:"version_id,omitempty"`
	ContentSHA256 string `json:"content_sha256,omitempty"`
	Publisher string `json:"publisher,omitempty"`
	URL string `json:"url,omitempty"`
	SourceType capevent.SourceType `json:"source_type,omitempty"`
	Authority capevent.EvidenceAuthority `json:"authority,omitempty"`
	IsPrimary bool `json:"is_primary"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	AvailableAt *time.Time `json:"available_at,omitempty"`
	FirstSeenAt *time.Time `json:"first_seen_at,omitempty"`
}

// safeEvidenceURL does not synthesize a missing link or expose a URL
// containing userinfo, search tokens, or fragments. Absent means unknown;
// unlike a reconstructed URL it makes no promise that a link will resolve.
func safeEvidenceURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Opaque != "" || u.Port() != "" && u.Port() != "443" ||
		strings.ContainsAny(u.String(), "\r\n\x00") {
		return ""
	}
	return u.String()
}

func indexCitedDocumentSources(result *Context, docs []capevent.CanonicalDocument) ([]CitedDocumentSource, error) {
	if result == nil { return nil, errors.New("event context is missing") }
	cited := make(map[string]struct{})
	for _, event := range result.Events {
		for _, id := range event.EvidenceDocumentIDs {
			if id == "" { return nil, errors.New("empty cited document id") }
			cited[id] = struct{}{}
		}
	}
	if len(cited) == 0 { return nil, nil }
	available := make(map[string]capevent.CanonicalDocument, len(docs))
	for _, doc := range docs {
		if doc.ID == "" { return nil, errors.New("missing canonical document id") }
		if _, duplicate := available[doc.ID]; duplicate {
			return nil, fmt.Errorf("ambiguous canonical document %q", doc.ID)
		}
		available[doc.ID] = doc
	}
	resultSources := make([]CitedDocumentSource, 0, len(cited))
	for id := range cited {
		doc, exists := available[id]
		if !exists { return nil, fmt.Errorf("cited canonical document %q is missing", id) }
		if doc.AvailableAt == nil || doc.AvailableAt.IsZero() ||
			doc.ContentHash == "" || len(doc.ContentHash) != 64 {
			return nil, fmt.Errorf("cited canonical document %q has no usable PIT/version identity", id)
		}
		src := CitedDocumentSource{
			DocumentID:id, VersionID:doc.VersionID, ContentSHA256:doc.ContentHash,
			Publisher:doc.Source.Publisher, URL:safeEvidenceURL(doc.Source.URL),
			SourceType:doc.Source.SourceType, Authority:doc.Source.Authority,
			IsPrimary:doc.Source.IsPrimary, PublishedAt:doc.PublishedAt,
			AvailableAt:doc.AvailableAt, FirstSeenAt:doc.FirstSeenAt,
		}
		resultSources = append(resultSources,src)
	}
	sort.Slice(resultSources,func(i,j int)bool{return resultSources[i].DocumentID<resultSources[j].DocumentID})
	return resultSources,nil
}
