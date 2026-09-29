package event

import (
	"testing"
	"time"
)

func TestCanonicalizer(t *testing.T) {
	asOf := time.Date(2026, 9, 24, 15, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	before := asOf.Add(-time.Hour)
	after := asOf.Add(time.Hour)

	t.Run("rejects future documents for PIT-safe task", func(t *testing.T) {
		task := ResearchTask{
			ID:   "task-1",
			AsOf: asOf,
			Requirements: ResearchRequirements{
				PITSafe: true,
			},
		}

		result, err := (Canonicalizer{}).Canonicalize(task, []RawDocument{
			{
				ID:          "raw-before",
				AttemptID:   "attempt-1",
				Title:       "before",
				Content:     "known before as-of",
				Publisher:   "SSE",
				PublishedAt: &before,
			},
			{
				ID:          "raw-after",
				AttemptID:   "attempt-2",
				Title:       "after",
				Content:     "future information",
				Publisher:   "SSE",
				PublishedAt: &after,
			},
		})
		if err != nil {
			t.Fatalf("Canonicalize() error = %v", err)
		}

		if len(result.Documents) != 1 {
			t.Fatalf("documents = %d, want 1", len(result.Documents))
		}
		if result.Documents[0].Title != "before" {
			t.Fatalf("document title = %q, want before", result.Documents[0].Title)
		}
		if len(result.Rejected) != 1 {
			t.Fatalf("rejected = %d, want 1", len(result.Rejected))
		}
		if result.Rejected[0].Reason != RejectionFuturePublishedAt {
			t.Fatalf("rejection reason = %q", result.Rejected[0].Reason)
		}
	})

	t.Run("rejects missing published_at for PIT-safe task", func(t *testing.T) {
		task := ResearchTask{
			ID:   "task-1",
			AsOf: asOf,
			Requirements: ResearchRequirements{
				PITSafe: true,
			},
		}

		result, err := (Canonicalizer{}).Canonicalize(task, []RawDocument{
			{
				ID:        "raw-1",
				AttemptID: "attempt-1",
				Title:     "unknown time",
				Content:   "cannot prove PIT safety",
				Publisher: "SSE",
			},
		})
		if err != nil {
			t.Fatalf("Canonicalize() error = %v", err)
		}

		if len(result.Documents) != 0 {
			t.Fatalf("documents = %d, want 0", len(result.Documents))
		}
		if len(result.Rejected) != 1 {
			t.Fatalf("rejected = %d, want 1", len(result.Rejected))
		}
		if result.Rejected[0].Reason != RejectionMissingPublishedAt {
			t.Fatalf("rejection reason = %q", result.Rejected[0].Reason)
		}
	})

	t.Run("merges the same document across acquisition attempts", func(t *testing.T) {
		task := ResearchTask{
			ID:   "task-1",
			AsOf: asOf,
			Requirements: ResearchRequirements{
				PITSafe: true,
			},
		}

		result, err := (Canonicalizer{}).Canonicalize(task, []RawDocument{
			{
				ID:          "raw-api",
				AttemptID:   "attempt-api",
				Title:       "贵州茅台公告",
				Content:     "same official disclosure",
				URL:         "https://example.com/disclosure/1",
				Publisher:   "上海证券交易所",
				SourceType:  SourceOfficial,
				IsPrimary:   true,
				PublishedAt: &before,
			},
			{
				ID:          "raw-search",
				AttemptID:   "attempt-search",
				Title:       "贵州茅台公告",
				Content:     "same official disclosure",
				URL:         "https://example.com/disclosure/1",
				Publisher:   "上海证券交易所",
				SourceType:  SourceOfficial,
				IsPrimary:   true,
				PublishedAt: &before,
			},
		})
		if err != nil {
			t.Fatalf("Canonicalize() error = %v", err)
		}

		if len(result.Documents) != 1 {
			t.Fatalf("documents = %d, want 1", len(result.Documents))
		}

		doc := result.Documents[0]
		if len(doc.RawDocumentIDs) != 2 {
			t.Fatalf("raw document refs = %v, want 2 refs", doc.RawDocumentIDs)
		}
		if len(doc.AttemptIDs) != 2 {
			t.Fatalf("attempt refs = %v, want 2 refs", doc.AttemptIDs)
		}
		if !doc.Source.IsPrimary {
			t.Fatal("canonical source should remain primary")
		}
	})
}
