package event

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileEvidenceSnapshotStorePersistsAndQueriesPITVersions(
	t *testing.T,
) {
	root := t.TempDir()
	store, err := NewFileEvidenceSnapshotStore(root)
	if err != nil {
		t.Fatalf(
			"NewFileEvidenceSnapshotStore() error = %v",
			err,
		)
	}

	published := time.Date(
		2026, 9, 29, 9, 0, 0, 0, time.UTC,
	)
	firstObservation := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)
	secondObservation := time.Date(
		2026, 9, 29, 11, 0, 0, 0, time.UTC,
	)

	first := RawDocument{
		ID:           "cls_news_100",
		Title:        "event",
		Content:      "same body",
		URL:          "https://www.cls.cn/detail/100",
		Publisher:    clsPublisher,
		SourceType:   SourceIndustryMedia,
		Authority:    AuthorityMedia,
		EvidenceKind: EvidenceNewsReport,
		PublishedAt:  &published,
		FirstSeenAt:  &firstObservation,
		AvailableAt:  &firstObservation,
		RetrievedAt:  firstObservation,
	}
	second := first
	second.FirstSeenAt = &secondObservation
	second.AvailableAt = &firstObservation
	second.RetrievedAt = secondObservation

	if err := store.Save(
		context.Background(),
		[]RawDocument{first, second},
	); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Saving the same immutable observations again must be idempotent.
	if err := store.Save(
		context.Background(),
		[]RawDocument{first, second},
	); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	files := snapshotJSONFiles(t, root)
	if len(files) != 2 {
		t.Fatalf(
			"snapshot files = %d, want 2 observations; files=%v",
			len(files),
			files,
		)
	}

	beforeAvailable, err := store.Query(
		context.Background(),
		EvidenceSnapshotQuery{
			AsOf: firstObservation.Add(-time.Second),
		},
	)
	if err != nil {
		t.Fatalf("Query(before) error = %v", err)
	}
	if len(beforeAvailable) != 0 {
		t.Fatalf(
			"documents before availability = %d, want 0",
			len(beforeAvailable),
		)
	}

	afterAvailable, err := store.Query(
		context.Background(),
		EvidenceSnapshotQuery{
			AsOf: secondObservation.Add(time.Hour),
		},
	)
	if err != nil {
		t.Fatalf("Query(after) error = %v", err)
	}
	if len(afterAvailable) != 1 {
		t.Fatalf(
			"documents after availability = %d, want one deduplicated version",
			len(afterAvailable),
		)
	}
	if !afterAvailable[0].RetrievedAt.Equal(
		firstObservation,
	) {
		t.Fatalf(
			"returned observation = %s, want earliest %s",
			afterAvailable[0].RetrievedAt,
			firstObservation,
		)
	}
}

func TestFileEvidenceSnapshotStoreKeepsDifferentVersions(
	t *testing.T,
) {
	store, err := NewFileEvidenceSnapshotStore(
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf(
			"NewFileEvidenceSnapshotStore() error = %v",
			err,
		)
	}

	published := time.Date(
		2026, 9, 29, 9, 0, 0, 0, time.UTC,
	)
	v1At := published.Add(time.Hour)
	v2At := published.Add(2 * time.Hour)

	v1 := RawDocument{
		ID:          "news-versioned",
		Title:       "article",
		Content:     "version one",
		URL:         "https://example.com/article/1",
		Publisher:   "media",
		SourceType:  SourceIndustryMedia,
		Authority:   AuthorityMedia,
		EvidenceKind: EvidenceNewsReport,
		PublishedAt: &published,
		AvailableAt: &v1At,
		RetrievedAt: v1At,
	}
	v2 := v1
	v2.Content = "version two"
	v2.UpdatedAt = &v2At
	v2.AvailableAt = &v2At
	v2.RetrievedAt = v2At

	if err := store.Save(
		context.Background(),
		[]RawDocument{v1, v2},
	); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	documents, err := store.Query(
		context.Background(),
		EvidenceSnapshotQuery{
			AsOf: v2At.Add(time.Hour),
		},
	)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(documents) != 2 {
		t.Fatalf(
			"version documents = %d, want 2",
			len(documents),
		)
	}

	canonical, err := (Canonicalizer{}).Canonicalize(
		ResearchTask{
			ID:   "replay-versioned",
			AsOf: v2At.Add(time.Hour),
			Requirements: ResearchRequirements{
				PITSafe: true,
			},
		},
		documents,
	)
	if err != nil {
		t.Fatalf(
			"Canonicalize() error = %v",
			err,
		)
	}
	if len(canonical.Documents) != 1 {
		t.Fatalf(
			"logical documents = %d, want 1",
			len(canonical.Documents),
		)
	}
	if len(canonical.Documents[0].Versions) != 2 {
		t.Fatalf(
			"versions = %d, want 2",
			len(canonical.Documents[0].Versions),
		)
	}
	if canonical.Documents[0].Content != "version two" {
		t.Fatalf(
			"active content = %q, want version two",
			canonical.Documents[0].Content,
		)
	}
}

func TestFileEvidenceSnapshotStoreQueryFilters(
	t *testing.T,
) {
	store, err := NewFileEvidenceSnapshotStore(
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf(
			"NewFileEvidenceSnapshotStore() error = %v",
			err,
		)
	}

	available := time.Date(
		2026, 9, 29, 10, 0, 0, 0, time.UTC,
	)
	cninfo := RawDocument{
		ID:           "cninfo_1",
		Title:        "issuer disclosure",
		URL:          "https://static.cninfo.com.cn/a.pdf",
		Publisher:    cninfoPublisher,
		SourceType:   SourceOfficial,
		Authority:    AuthorityIssuer,
		EvidenceKind: EvidenceDisclosure,
		IsPrimary:    true,
		PublishedAt:  &available,
		AvailableAt:  &available,
		RetrievedAt:  available.Add(time.Hour),
		RawMetadata: map[string]any{
			"symbol":        "600519",
			"instrument_id": "600519.SH",
		},
	}
	cls := RawDocument{
		ID:           "cls_1",
		Title:        "media report",
		URL:          "https://www.cls.cn/detail/1",
		Publisher:    clsPublisher,
		SourceType:   SourceIndustryMedia,
		Authority:    AuthorityMedia,
		EvidenceKind: EvidenceNewsReport,
		PublishedAt:  &available,
		AvailableAt:  &available,
		RetrievedAt:  available,
	}

	if err := store.Save(
		context.Background(),
		[]RawDocument{cninfo, cls},
	); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	documents, err := store.Query(
		context.Background(),
		EvidenceSnapshotQuery{
			AsOf:   available.Add(2 * time.Hour),
			Symbol: "600519.SH",
			SourceTypes: []SourceType{
				SourceOfficial,
			},
			Authorities: []EvidenceAuthority{
				AuthorityIssuer,
			},
			EvidenceKinds: []EvidenceKind{
				EvidenceDisclosure,
			},
			TimeRange: TimeRange{
				Start: available.Add(-time.Minute),
				End:   available.Add(time.Minute),
			},
		},
	)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(documents) != 1 ||
		documents[0].ID != cninfo.ID {
		t.Fatalf(
			"filtered documents = %+v, want Cninfo only",
			documents,
		)
	}

	missing, err := store.Query(
		context.Background(),
		EvidenceSnapshotQuery{
			AsOf:   available.Add(2 * time.Hour),
			Symbol: "000001.SZ",
		},
	)
	if err != nil {
		t.Fatalf("Query(missing symbol) error = %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf(
			"missing-symbol documents = %d, want 0",
			len(missing),
		)
	}
}

func TestFileEvidenceSnapshotStoreSourcePolicies(
	t *testing.T,
) {
	t.Run("static Cninfo can replay from publication time", func(t *testing.T) {
		store, err := NewFileEvidenceSnapshotStore(
			t.TempDir(),
		)
		if err != nil {
			t.Fatal(err)
		}

		published := time.Date(
			2026, 9, 29, 10, 0, 0, 0, time.UTC,
		)
		retrieved := published.Add(24 * time.Hour)
		document := cninfoRawDocument(
			ResearchTask{ID: "cninfo-store"},
			cninfoStock{
				Symbol:       "600519",
				InstrumentID: "600519.SH",
				Exchange:     "SSE",
				Name:         "贵州茅台",
			},
			map[string]any{
				"announcementId":    "store-static",
				"announcementTitle": "公告",
				"announcementTime":  published.UnixMilli(),
				"adjunctUrl":         "finalpage/store-static.PDF",
			},
			retrieved,
		)
		if document == nil {
			t.Fatal("cninfoRawDocument() returned nil")
		}

		if err := store.Save(
			context.Background(),
			[]RawDocument{*document},
		); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		replayed, err := store.Query(
			context.Background(),
			EvidenceSnapshotQuery{
				AsOf: published.Add(time.Hour),
			},
		)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}
		if len(replayed) != 1 {
			t.Fatalf(
				"replayed documents = %d, want 1",
				len(replayed),
			)
		}
	})

	t.Run("mutable CLS waits until observed version availability", func(t *testing.T) {
		store, err := NewFileEvidenceSnapshotStore(
			t.TempDir(),
		)
		if err != nil {
			t.Fatal(err)
		}

		published := time.Date(
			2026, 9, 29, 10, 0, 0, 0, time.UTC,
		)
		retrieved := published.Add(24 * time.Hour)
		document := clsRawDocument(
			CLSNewsImportant,
			clsNewsItem{
				ID:      "snapshot-cls",
				Title:   "历史新闻",
				Content: "实际观察到的正文",
				Ctime:   published.Unix(),
			},
			retrieved,
		)
		if document == nil {
			t.Fatal("clsRawDocument() returned nil")
		}

		if err := store.Save(
			context.Background(),
			[]RawDocument{*document},
		); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		before, err := store.Query(
			context.Background(),
			EvidenceSnapshotQuery{
				AsOf: published.Add(time.Hour),
			},
		)
		if err != nil {
			t.Fatalf("Query(before) error = %v", err)
		}
		if len(before) != 0 {
			t.Fatalf(
				"documents before CLS observation = %d, want 0",
				len(before),
			)
		}

		after, err := store.Query(
			context.Background(),
			EvidenceSnapshotQuery{
				AsOf: retrieved.Add(time.Hour),
			},
		)
		if err != nil {
			t.Fatalf("Query(after) error = %v", err)
		}
		if len(after) != 1 {
			t.Fatalf(
				"documents after CLS observation = %d, want 1",
				len(after),
			)
		}
	})
}

func TestFileEvidenceSnapshotStoreRejectsCorruptRecord(
	t *testing.T,
) {
	root := t.TempDir()
	store, err := NewFileEvidenceSnapshotStore(root)
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(
		root,
		evidenceSnapshotDirectory,
		"doc_corrupt",
		"ver_corrupt",
	)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "snapshot_corrupt.json"),
		[]byte("{not-json"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	_, err = store.Query(
		context.Background(),
		EvidenceSnapshotQuery{
			AsOf: time.Now().UTC(),
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"decode evidence snapshot",
		) {
		t.Fatalf(
			"error = %v, want corrupt snapshot error",
			err,
		)
	}
}

func TestFileEvidenceSnapshotStoreHonorsContext(
	t *testing.T,
) {
	store, err := NewFileEvidenceSnapshotStore(
		t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	err = store.Save(
		ctx,
		[]RawDocument{
			{
				ID:          "cancelled",
				RetrievedAt: time.Now().UTC(),
			},
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"Save() error = %v, want context.Canceled",
			err,
		)
	}
}

func snapshotJSONFiles(
	t *testing.T,
	root string,
) []string {
	t.Helper()

	files := make([]string, 0)
	err := filepath.WalkDir(
		filepath.Join(
			root,
			evidenceSnapshotDirectory,
		),
		func(
			path string,
			entry os.DirEntry,
			err error,
		) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() &&
				filepath.Ext(entry.Name()) == ".json" {
				files = append(files, path)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf(
			"WalkDir() error = %v",
			err,
		)
	}
	return files
}
