package event

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
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
	indexByLogicalKey := make(
		map[string]int,
		len(rawDocuments),
	)

	for _, raw := range rawDocuments {
		raw.ID = strings.TrimSpace(raw.ID)
		if raw.ID == "" {
			return CanonicalizationResult{}, errors.New(
				"event raw document id is required",
			)
		}

		availableAt := effectiveAvailableAt(raw)

		if task.Requirements.PITSafe {
			switch {
			case raw.PublishedAt != nil &&
				raw.PublishedAt.After(task.AsOf):
				result.Rejected = append(
					result.Rejected,
					RejectedDocument{
						RawDocumentID: raw.ID,
						Reason:        RejectionFuturePublishedAt,
					},
				)
				continue

			case raw.UpdatedAt != nil &&
				raw.UpdatedAt.After(task.AsOf):
				result.Rejected = append(
					result.Rejected,
					RejectedDocument{
						RawDocumentID: raw.ID,
						Reason:        RejectionFutureUpdatedAt,
					},
				)
				continue

			case availableAt == nil:
				result.Rejected = append(
					result.Rejected,
					RejectedDocument{
						RawDocumentID: raw.ID,
						Reason:        RejectionMissingAvailableAt,
					},
				)
				continue

			case availableAt.After(task.AsOf):
				result.Rejected = append(
					result.Rejected,
					RejectedDocument{
						RawDocumentID: raw.ID,
						Reason:        RejectionFutureAvailableAt,
					},
				)
				continue
			}
		}

		logicalKey := canonicalLogicalDocumentKey(raw)
		if logicalKey == "" {
			return CanonicalizationResult{}, errors.New(
				"event canonical document identity is required",
			)
		}

		doc := canonicalDocumentFromRaw(
			raw,
			logicalKey,
			availableAt,
		)

		if index, ok := indexByLogicalKey[logicalKey]; ok {
			mergeCanonicalDocument(
				&result.Documents[index],
				doc,
			)
			continue
		}

		result.Documents = append(
			result.Documents,
			doc,
		)
		indexByLogicalKey[logicalKey] =
			len(result.Documents) - 1
	}

	for index := range result.Documents {
		finalizeCanonicalDocument(
			&result.Documents[index],
		)
	}

	return result, nil
}

func canonicalDocumentFromRaw(
	raw RawDocument,
	logicalKey string,
	availableAt *time.Time,
) CanonicalDocument {
	publisher := strings.TrimSpace(raw.Publisher)
	logicalID := canonicalDocumentID(logicalKey)

	version := documentVersionFromRaw(
		raw,
		logicalID,
		logicalKey,
		availableAt,
	)

	doc := CanonicalDocument{
		ID: logicalID,
		Source: DocumentSource{
			Publisher:    publisher,
			URL:          normalizeDocumentURL(raw.URL),
			SourceType:       raw.SourceType,
			Authority:        raw.Authority,
			EvidenceKind:     raw.EvidenceKind,
			IsPrimary:        raw.IsPrimary,
			CanonicalDomain:  strings.TrimSpace(raw.CanonicalDomain),
			ResolutionReason: strings.TrimSpace(raw.ResolutionReason),
		},
		Versions: []DocumentVersion{version},
	}
	mergeCanonicalReferences(
		&doc,
		version,
	)
	applyActiveVersion(
		&doc,
		version,
	)
	return doc
}

func documentVersionFromRaw(
	raw RawDocument,
	logicalID string,
	logicalKey string,
	availableAt *time.Time,
) DocumentVersion {
	content := strings.TrimSpace(raw.Content)
	firstSeenAt := cloneTime(raw.FirstSeenAt)
	if firstSeenAt == nil &&
		!raw.RetrievedAt.IsZero() {
		value := raw.RetrievedAt
		firstSeenAt = &value
	}

	version := DocumentVersion{
		LogicalDocumentID: logicalID,
		Title:             strings.TrimSpace(raw.Title),
		Content:           content,
		ContentHash:       contentHash(content),
		PublishedAt:       cloneTime(raw.PublishedAt),
		UpdatedAt:         cloneTime(raw.UpdatedAt),
		FirstSeenAt:       firstSeenAt,
		AvailableAt:       cloneTime(availableAt),
		SnapshotAt:        raw.RetrievedAt,
		RawDocumentIDs:    appendUnique(nil, raw.ID),
		AttemptIDs: appendUnique(
			nil,
			strings.TrimSpace(raw.AttemptID),
		),
	}
	version.ID = documentVersionID(
		logicalKey,
		version,
	)
	return version
}

func canonicalLogicalDocumentKey(
	raw RawDocument,
) string {
	if canonicalURL := strings.ToLower(
		strings.TrimSpace(
			normalizeDocumentURL(raw.URL),
		),
	); canonicalURL != "" {
		return "url\x00" + canonicalURL
	}

	rawID := strings.ToLower(
		strings.TrimSpace(raw.ID),
	)
	if rawID != "" {
		return "raw\x00" + rawID
	}

	publisher := strings.ToLower(
		strings.TrimSpace(raw.Publisher),
	)
	title := strings.ToLower(
		strings.TrimSpace(raw.Title),
	)
	if title != "" {
		return publisher + "\x00title\x00" + title
	}

	if hash := contentHash(
		strings.TrimSpace(raw.Content),
	); hash != "" {
		return publisher + "\x00content\x00" + hash
	}

	return ""
}

func canonicalDocumentID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "doc_" + hex.EncodeToString(sum[:8])
}

func documentVersionID(
	logicalKey string,
	version DocumentVersion,
) string {
	identityParts := make([]string, 0, 3)

	if payloadHash := versionPayloadHash(
		version.Title,
		version.Content,
	); payloadHash != "" {
		identityParts = append(
			identityParts,
			"payload="+payloadHash,
		)
	}

	if version.UpdatedAt != nil {
		identityParts = append(
			identityParts,
			"updated="+version.UpdatedAt.UTC().
				Format(time.RFC3339Nano),
		)
	}

	if len(identityParts) == 0 &&
		version.PublishedAt != nil {
		identityParts = append(
			identityParts,
			"published="+version.PublishedAt.UTC().
				Format(time.RFC3339Nano),
		)
	}
	if len(identityParts) == 0 &&
		version.AvailableAt != nil {
		identityParts = append(
			identityParts,
			"available="+version.AvailableAt.UTC().
				Format(time.RFC3339Nano),
		)
	}
	if len(identityParts) == 0 &&
		!version.SnapshotAt.IsZero() {
		identityParts = append(
			identityParts,
			"snapshot="+version.SnapshotAt.UTC().
				Format(time.RFC3339Nano),
		)
	}
	if len(identityParts) == 0 {
		identityParts = append(
			identityParts,
			"unknown",
		)
	}

	sum := sha256.Sum256(
		[]byte(
			logicalKey +
				"\x00version\x00" +
				strings.Join(identityParts, "\x00"),
		),
	)
	return "ver_" + hex.EncodeToString(sum[:8])
}

func versionPayloadHash(
	title string,
	content string,
) string {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" && content == "" {
		return ""
	}

	sum := sha256.Sum256(
		[]byte(title + "\x00" + content),
	)
	return hex.EncodeToString(sum[:])
}

func effectiveAvailableAt(
	raw RawDocument,
) *time.Time {
	if raw.AvailableAt == nil {
		return nil
	}

	effective := *raw.AvailableAt
	if raw.PublishedAt != nil &&
		raw.PublishedAt.After(effective) {
		effective = *raw.PublishedAt
	}
	if raw.UpdatedAt != nil &&
		raw.UpdatedAt.After(effective) {
		effective = *raw.UpdatedAt
	}
	return &effective
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

	mergeDocumentSource(
		&dst.Source,
		src.Source,
	)

	for _, srcVersion := range src.Versions {
		versionIndex := -1
		for index := range dst.Versions {
			if dst.Versions[index].ID ==
				srcVersion.ID {
				versionIndex = index
				break
			}
		}

		if versionIndex >= 0 {
			mergeDocumentVersion(
				&dst.Versions[versionIndex],
				srcVersion,
			)
		} else {
			dst.Versions = append(
				dst.Versions,
				srcVersion,
			)
		}
		mergeCanonicalReferences(
			dst,
			srcVersion,
		)
	}
}

func mergeDocumentSource(
	dst *DocumentSource,
	src DocumentSource,
) {
	if dst == nil {
		return
	}

	if dst.Publisher == "" {
		dst.Publisher = src.Publisher
	}
	if dst.URL == "" {
		dst.URL = src.URL
	}
	if dst.SourceType == "" {
		dst.SourceType = src.SourceType
	}
	if dst.Authority == "" {
		dst.Authority = src.Authority
	}
	if dst.EvidenceKind == "" {
		dst.EvidenceKind = src.EvidenceKind
	}
	if dst.CanonicalDomain == "" {
		dst.CanonicalDomain = src.CanonicalDomain
	}
	if dst.ResolutionReason == "" {
		dst.ResolutionReason = src.ResolutionReason
	}
	dst.IsPrimary = dst.IsPrimary ||
		src.IsPrimary
}

func mergeDocumentVersion(
	dst *DocumentVersion,
	src DocumentVersion,
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

	dst.PublishedAt = earlierTime(
		dst.PublishedAt,
		src.PublishedAt,
	)
	dst.UpdatedAt = earlierTime(
		dst.UpdatedAt,
		src.UpdatedAt,
	)
	dst.FirstSeenAt = earlierTime(
		dst.FirstSeenAt,
		src.FirstSeenAt,
	)
	dst.AvailableAt = earlierTime(
		dst.AvailableAt,
		src.AvailableAt,
	)

	switch {
	case dst.SnapshotAt.IsZero():
		dst.SnapshotAt = src.SnapshotAt
	case !src.SnapshotAt.IsZero() &&
		src.SnapshotAt.Before(dst.SnapshotAt):
		dst.SnapshotAt = src.SnapshotAt
	}
}

func mergeCanonicalReferences(
	doc *CanonicalDocument,
	version DocumentVersion,
) {
	if doc == nil {
		return
	}

	doc.RawDocumentIDs = appendUnique(
		doc.RawDocumentIDs,
		version.RawDocumentIDs...,
	)
	doc.AttemptIDs = appendUnique(
		doc.AttemptIDs,
		version.AttemptIDs...,
	)
}

func finalizeCanonicalDocument(
	doc *CanonicalDocument,
) {
	if doc == nil ||
		len(doc.Versions) == 0 {
		return
	}

	sort.SliceStable(
		doc.Versions,
		func(i int, j int) bool {
			return compareDocumentVersions(
				doc.Versions[i],
				doc.Versions[j],
			) < 0
		},
	)

	active := doc.Versions[len(doc.Versions)-1]
	applyActiveVersion(
		doc,
		active,
	)
}

func compareDocumentVersions(
	left DocumentVersion,
	right DocumentVersion,
) int {
	leftTime := versionOrderTime(left)
	rightTime := versionOrderTime(right)

	switch {
	case leftTime.Before(rightTime):
		return -1
	case rightTime.Before(leftTime):
		return 1
	case left.ID < right.ID:
		return -1
	case left.ID > right.ID:
		return 1
	default:
		return 0
	}
}

func versionOrderTime(
	version DocumentVersion,
) time.Time {
	switch {
	case version.AvailableAt != nil:
		return *version.AvailableAt
	case version.UpdatedAt != nil:
		return *version.UpdatedAt
	case version.PublishedAt != nil:
		return *version.PublishedAt
	case version.FirstSeenAt != nil:
		return *version.FirstSeenAt
	default:
		return version.SnapshotAt
	}
}

func applyActiveVersion(
	doc *CanonicalDocument,
	version DocumentVersion,
) {
	if doc == nil {
		return
	}

	doc.VersionID = version.ID
	doc.Title = version.Title
	doc.Content = version.Content
	doc.ContentHash = version.ContentHash
	doc.PublishedAt = cloneTime(
		version.PublishedAt,
	)
	doc.UpdatedAt = cloneTime(
		version.UpdatedAt,
	)
	doc.FirstSeenAt = cloneTime(
		version.FirstSeenAt,
	)
	doc.AvailableAt = cloneTime(
		version.AvailableAt,
	)
}

func earlierTime(
	left *time.Time,
	right *time.Time,
) *time.Time {
	switch {
	case left == nil:
		return cloneTime(right)
	case right == nil:
		return cloneTime(left)
	case right.Before(*left):
		return cloneTime(right)
	default:
		return cloneTime(left)
	}
}

func appendUnique(
	values []string,
	candidates ...string,
) []string {
	seen := make(
		map[string]struct{},
		len(values)+len(candidates),
	)
	for _, value := range values {
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(
			candidate,
		)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		values = append(
			values,
			candidate,
		)
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
