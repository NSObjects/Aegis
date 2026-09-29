package event

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"
)

type Canonicalizer struct{}

func (Canonicalizer) Canonicalize(
	task ResearchTask,
	rawDocuments []RawDocument,
) (CanonicalizationResult, error) {
	if task.Requirements.PITSafe && task.AsOf.IsZero() {
		return CanonicalizationResult{}, errors.New(
			"event research as_of is required for PIT-safe canonicalization",
		)
	}

	result := CanonicalizationResult{
		Documents: make([]CanonicalDocument, 0, len(rawDocuments)),
		Rejected:  make([]RejectedDocument, 0),
	}
	indexByKey := make(map[string]int, len(rawDocuments))

	for _, raw := range rawDocuments {
		raw.ID = strings.TrimSpace(raw.ID)
		if raw.ID == "" {
			return CanonicalizationResult{}, errors.New(
				"event raw document id is required",
			)
		}

		if task.Requirements.PITSafe {
			if raw.PublishedAt == nil {
				result.Rejected = append(
					result.Rejected,
					RejectedDocument{
						RawDocumentID: raw.ID,
						Reason:        RejectionMissingPublishedAt,
					},
				)
				continue
			}
			if raw.PublishedAt.After(task.AsOf) {
				result.Rejected = append(
					result.Rejected,
					RejectedDocument{
						RawDocumentID: raw.ID,
						Reason:        RejectionFuturePublishedAt,
					},
				)
				continue
			}
		}

		doc := canonicalDocumentFromRaw(raw)
		key := canonicalDocumentKey(doc)

		if index, ok := indexByKey[key]; ok {
			mergeCanonicalDocument(
				&result.Documents[index],
				doc,
			)
			continue
		}

		doc.ID = canonicalDocumentID(key)
		result.Documents = append(result.Documents, doc)
		indexByKey[key] = len(result.Documents) - 1
	}

	return result, nil
}

func canonicalDocumentFromRaw(raw RawDocument) CanonicalDocument {
	content := strings.TrimSpace(raw.Content)
	publisher := strings.TrimSpace(raw.Publisher)

	return CanonicalDocument{
		Title:   strings.TrimSpace(raw.Title),
		Content: content,
		Source: DocumentSource{
			Publisher:  publisher,
			URL:        normalizeDocumentURL(raw.URL),
			SourceType: raw.SourceType,
			IsPrimary:  raw.IsPrimary,
		},
		PublishedAt:    cloneTime(raw.PublishedAt),
		ContentHash:    contentHash(content),
		RawDocumentIDs: appendUnique(nil, raw.ID),
		AttemptIDs:     appendUnique(nil, strings.TrimSpace(raw.AttemptID)),
	}
}

func canonicalDocumentKey(doc CanonicalDocument) string {
	if canonicalURL := strings.ToLower(
		strings.TrimSpace(doc.Source.URL),
	); canonicalURL != "" {
		return "url\x00" + canonicalURL
	}

	sourceIdentity := strings.ToLower(
		strings.TrimSpace(doc.Source.Publisher),
	)
	if doc.ContentHash != "" {
		return sourceIdentity + "\x00content\x00" + doc.ContentHash
	}

	return sourceIdentity +
		"\x00title\x00" + strings.ToLower(strings.TrimSpace(doc.Title))
}

func canonicalDocumentID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "doc_" + hex.EncodeToString(sum[:8])
}

func contentHash(content string) string {
	if content == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func normalizeDocumentURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	parsed.Fragment = ""
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String()
}

func mergeCanonicalDocument(
	dst *CanonicalDocument,
	src CanonicalDocument,
) {
	if dst == nil {
		return
	}

	dst.RawDocumentIDs = appendUnique(
		dst.RawDocumentIDs,
		src.RawDocumentIDs...,
	)
	dst.AttemptIDs = appendUnique(
		dst.AttemptIDs,
		src.AttemptIDs...,
	)

	if dst.Title == "" {
		dst.Title = src.Title
	}
	if dst.Content == "" {
		dst.Content = src.Content
	}
	if dst.ContentHash == "" {
		dst.ContentHash = src.ContentHash
	}
	if dst.Source.Publisher == "" {
		dst.Source.Publisher = src.Source.Publisher
	}
	if dst.Source.URL == "" {
		dst.Source.URL = src.Source.URL
	}
	if dst.Source.SourceType == "" {
		dst.Source.SourceType = src.Source.SourceType
	}
	dst.Source.IsPrimary = dst.Source.IsPrimary || src.Source.IsPrimary

	switch {
	case dst.PublishedAt == nil:
		dst.PublishedAt = cloneTime(src.PublishedAt)
	case src.PublishedAt != nil && src.PublishedAt.Before(*dst.PublishedAt):
		dst.PublishedAt = cloneTime(src.PublishedAt)
	}
}

func appendUnique(
	values []string,
	candidates ...string,
) []string {
	seen := make(map[string]struct{}, len(values)+len(candidates))
	for _, value := range values {
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		values = append(values, candidate)
	}

	return values
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
