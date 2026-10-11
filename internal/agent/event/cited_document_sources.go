package event

import (
	"errors"
	"encoding/hex"
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

func validateCitedDocumentSources(ctx *Context) error {
	if len(ctx.CitedDocumentSources) == 0 { return nil }
	if ctx.Status != StatusNormal { return errors.New("only NORMAL event context can cite documents") }
	ids := make(map[string]struct{})
	for _, event := range ctx.Events {
		for _, id := range event.EvidenceDocumentIDs { ids[id] = struct{}{} }
	}
	if len(ids) != len(ctx.CitedDocumentSources) {
		return errors.New("cited source index does not cover all event document identities")
	}
	var previous string
	for _, item := range ctx.CitedDocumentSources {
		if item.DocumentID == "" || (previous != "" && item.DocumentID <= previous) {
			return errors.New("cited source identities must be unique and sorted")
		}
		if _, ok := ids[item.DocumentID]; !ok {
			return errors.New("cited source has no referenced event")
		}
		if item.VersionID == "" || len(item.ContentSHA256) != 64 {
			return errors.New("cited source missing canonical version or SHA256")
		}
		if _, err := hex.DecodeString(item.ContentSHA256); err != nil ||
			strings.ToLower(item.ContentSHA256) != item.ContentSHA256 {
			return errors.New("invalid cited source content digest")
		}
		if item.AvailableAt == nil || item.AvailableAt.IsZero() {
			return errors.New("cited source missing observed availability")
		}
		if item.PublishedAt != nil && item.PublishedAt.After(*item.AvailableAt) {
			return errors.New("cited source was published after recorded availability")
		}
		if item.URL != "" && safeEvidenceURL(item.URL) != item.URL {
			return errors.New("unsafe cited source URL")
		}
		for _, value := range []string{item.DocumentID,item.VersionID,item.Publisher} {
			if strings.ContainsAny(value,"\r\n\x00") || len(value)>512 {
				return errors.New("invalid cited source identity or publisher")
			}
		}
		previous=item.DocumentID
	}
	return nil
}
