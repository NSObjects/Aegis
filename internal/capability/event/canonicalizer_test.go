package event

import (
	"testing"
	"time"
)

func TestCanonicalizerPITAvailability(t *testing.T) {
	asOf := time.Date(
		2026, 9, 24, 15, 0, 0, 0,
		time.FixedZone("CST", 8*60*60),
	)
	before := asOf.Add(-time.Hour)
	after := asOf.Add(time.Hour)

	t.Run("rejects future published documents", func(t *testing.T) {
		result, err := (Canonicalizer{}).Canonicalize(
			ResearchTask{
				ID:   "task-future-published",
				AsOf: asOf,
				Requirements: ResearchRequirements{
					PITSafe: true,
				},
			},
			[]RawDocument{
				{
					ID:          "raw-before",
					Title:       "before",
					Content:     "known before as-of",
					PublishedAt: &before,
					AvailableAt: &before,
				},
				{
					ID:          "raw-after",
					Title:       "after",
					Content:     "future information",
					PublishedAt: &after,
					AvailableAt: &after,
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
		if len(result.Rejected) != 1 ||
			result.Rejected[0].Reason !=
				RejectionFuturePublishedAt {
			t.Fatalf(
				"rejected = %+v, want future-published rejection",
				result.Rejected,
			)
		}
	})

	t.Run("accepts missing published_at when availability is known", func(t *testing.T) {
		result, err := (Canonicalizer{}).Canonicalize(
			ResearchTask{
				ID:   "task-known-availability",
				AsOf: asOf,
				Requirements: ResearchRequirements{
					PITSafe: true,
				},
			},
			[]RawDocument{
				{
					ID:          "raw-known-availability",
					Title:       "observed source without publication metadata",
					Content:     "known content",
					AvailableAt: &before,
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
		if len(result.Rejected) != 0 {
			t.Fatalf(
				"rejected = %+v, want none",
				result.Rejected,
			)
		}
	})

	t.Run("rejects missing available_at", func(t *testing.T) {
		result, err := (Canonicalizer{}).Canonicalize(
			ResearchTask{
				ID:   "task-missing-availability",
				AsOf: asOf,
				Requirements: ResearchRequirements{
					PITSafe: true,
				},
			},
			[]RawDocument{
				{
					ID:          "raw-no-availability",
					Title:       "publication alone is insufficient",
					PublishedAt: &before,
				},
			},
		)
		if err != nil {
			t.Fatalf("Canonicalize() error = %v", err)
		}
		if len(result.Documents) != 0 {
			t.Fatalf(
				"documents = %d, want 0",
				len(result.Documents),
			)
		}
		if len(result.Rejected) != 1 ||
			result.Rejected[0].Reason !=
				RejectionMissingAvailableAt {
			t.Fatalf(
				"rejected = %+v, want missing-availability rejection",
				result.Rejected,
			)
		}
	})

	t.Run("rejects evidence version first available after as_of", func(t *testing.T) {
		result, err := (Canonicalizer{}).Canonicalize(
			ResearchTask{
				ID:   "task-future-availability",
				AsOf: asOf,
				Requirements: ResearchRequirements{
					PITSafe: true,
				},
			},
			[]RawDocument{
				{
					ID:          "raw-late-version",
					Title:       "old article current body",
					Content:     "body observed later",
					PublishedAt: &before,
					AvailableAt: &after,
				},
			},
		)
		if err != nil {
			t.Fatalf("Canonicalize() error = %v", err)
		}
		if len(result.Documents) != 0 {
			t.Fatalf(
				"documents = %d, want 0",
				len(result.Documents),
			)
		}
		if len(result.Rejected) != 1 ||
			result.Rejected[0].Reason !=
				RejectionFutureAvailableAt {
			t.Fatalf(
				"rejected = %+v, want future-availability rejection",
				result.Rejected,
			)
		}
	})

	t.Run("rejects version updated after as_of", func(t *testing.T) {
		result, err := (Canonicalizer{}).Canonicalize(
			ResearchTask{
				ID:   "task-future-update",
				AsOf: asOf,
				Requirements: ResearchRequirements{
					PITSafe: true,
				},
			},
			[]RawDocument{
				{
					ID:          "raw-updated-later",
					Title:       "edited article",
					Content:     "new body",
					PublishedAt: &before,
					UpdatedAt:   &after,
					AvailableAt: &before,
				},
			},
		)
		if err != nil {
			t.Fatalf("Canonicalize() error = %v", err)
		}
		if len(result.Documents) != 0 {
			t.Fatalf(
				"documents = %d, want 0",
				len(result.Documents),
			)
		}
		if len(result.Rejected) != 1 ||
			result.Rejected[0].Reason !=
				RejectionFutureUpdatedAt {
			t.Fatalf(
				"rejected = %+v, want future-updated rejection",
				result.Rejected,
			)
		}
	})
}

func TestCanonicalizerSeparatesLogicalDocumentAndVersions(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 24, 15, 0, 0, 0, time.UTC,
	)
	firstAvailable := asOf.Add(-2 * time.Hour)
	secondAvailable := asOf.Add(-time.Hour)
	firstSeen := firstAvailable.Add(time.Minute)
	secondSeen := secondAvailable.Add(time.Minute)

	result, err := (Canonicalizer{}).Canonicalize(
		ResearchTask{
			ID:   "task-versions",
			AsOf: asOf,
			Requirements: ResearchRequirements{
				PITSafe: true,
			},
		},
		[]RawDocument{
			{
				ID:          "raw-v1",
				AttemptID:   "attempt-v1",
				Title:       "article",
				Content:     "first body",
				URL:         "https://example.com/article/1",
				Publisher:   "publisher",
				PublishedAt: &firstAvailable,
				FirstSeenAt: &firstSeen,
				AvailableAt: &firstAvailable,
				RetrievedAt: firstSeen,
			},
			{
				ID:          "raw-v2",
				AttemptID:   "attempt-v2",
				Title:       "article updated",
				Content:     "second body",
				URL:         "https://example.com/article/1",
				Publisher:   "publisher",
				PublishedAt: &firstAvailable,
				UpdatedAt:   &secondAvailable,
				FirstSeenAt: &secondSeen,
				AvailableAt: &secondAvailable,
				RetrievedAt: secondSeen,
			},
		},
	)
	if err != nil {
		t.Fatalf("Canonicalize() error = %v", err)
	}
	if len(result.Documents) != 1 {
		t.Fatalf(
			"logical documents = %d, want 1",
			len(result.Documents),
		)
	}

	doc := result.Documents[0]
	if len(doc.Versions) != 2 {
		t.Fatalf(
			"versions = %d, want 2",
			len(doc.Versions),
		)
	}
	if doc.VersionID != doc.Versions[1].ID {
		t.Fatalf(
			"active version = %q, want latest %q",
			doc.VersionID,
			doc.Versions[1].ID,
		)
	}
	if doc.Content != "second body" {
		t.Fatalf(
			"active content = %q, want second body",
			doc.Content,
		)
	}
	if doc.AvailableAt == nil ||
		!doc.AvailableAt.Equal(secondAvailable) {
		t.Fatalf(
			"active available_at = %v, want %s",
			doc.AvailableAt,
			secondAvailable,
		)
	}
	if doc.ID != doc.Versions[0].LogicalDocumentID ||
		doc.ID != doc.Versions[1].LogicalDocumentID {
		t.Fatalf(
			"logical ids mismatch: doc=%q versions=%q/%q",
			doc.ID,
			doc.Versions[0].LogicalDocumentID,
			doc.Versions[1].LogicalDocumentID,
		)
	}
}

func TestCanonicalizerMergesSameVersionAcrossAttempts(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 24, 15, 0, 0, 0, time.UTC,
	)
	published := asOf.Add(-time.Hour)
	firstSeen := published.Add(time.Minute)
	laterSeen := published.Add(30 * time.Minute)

	result, err := (Canonicalizer{}).Canonicalize(
		ResearchTask{
			ID:   "task-same-version",
			AsOf: asOf,
			Requirements: ResearchRequirements{
				PITSafe: true,
			},
		},
		[]RawDocument{
			{
				ID:          "raw-api",
				AttemptID:   "attempt-api",
				Title:       "贵州茅台公告",
				Content:     "full official disclosure",
				URL:         "https://example.com/disclosure/1",
				Publisher:   "上海证券交易所",
				SourceType:  SourceOfficial,
				Authority:   AuthorityIssuer,
				EvidenceKind: EvidenceDisclosure,
				IsPrimary:   true,
				PublishedAt: &published,
				FirstSeenAt: &firstSeen,
				AvailableAt: &published,
				RetrievedAt: firstSeen,
			},
			{
				ID:          "raw-search",
				AttemptID:   "attempt-search",
				Title:       "贵州茅台公告",
				Content:     "full official disclosure",
				URL:         "https://example.com/disclosure/1",
				Publisher:   "SSE",
				SourceType:  SourceOfficial,
				Authority:   AuthorityIssuer,
				EvidenceKind: EvidenceDisclosure,
				IsPrimary:   true,
				PublishedAt: &published,
				FirstSeenAt: &laterSeen,
				AvailableAt: &published,
				RetrievedAt: laterSeen,
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
	if len(doc.Versions) != 1 {
		t.Fatalf(
			"versions = %d, want 1",
			len(doc.Versions),
		)
	}
	if len(doc.RawDocumentIDs) != 2 {
		t.Fatalf(
			"raw document refs = %v, want 2 refs",
			doc.RawDocumentIDs,
		)
	}
	if len(doc.AttemptIDs) != 2 {
		t.Fatalf(
			"attempt refs = %v, want 2 refs",
			doc.AttemptIDs,
		)
	}
	if doc.FirstSeenAt == nil ||
		!doc.FirstSeenAt.Equal(firstSeen) {
		t.Fatalf(
			"first_seen_at = %v, want earliest %s",
			doc.FirstSeenAt,
			firstSeen,
		)
	}
}
