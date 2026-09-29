package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	evidenceSnapshotDirectory     = "snapshots"
	evidenceSnapshotSchemaVersion = 1
)

type FileEvidenceSnapshotStore struct {
	root string
}

var _ EvidenceSnapshotStore = (*FileEvidenceSnapshotStore)(nil)

func NewFileEvidenceSnapshotStore(
	root string,
) (*FileEvidenceSnapshotStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New(
			"evidence snapshot store root is required",
		)
	}

	return &FileEvidenceSnapshotStore{
		root: filepath.Clean(root),
	}, nil
}

func (s *FileEvidenceSnapshotStore) Save(
	ctx context.Context,
	documents []RawDocument,
) error {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return errors.New(
			"evidence snapshot store is not initialized",
		)
	}

	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return err
		}

		snapshot, err := evidenceSnapshotFromRaw(
			document,
		)
		if err != nil {
			return err
		}

		if err := s.writeSnapshot(
			ctx,
			snapshot,
		); err != nil {
			return err
		}
	}

	return nil
}

func (s *FileEvidenceSnapshotStore) Query(
	ctx context.Context,
	query EvidenceSnapshotQuery,
) ([]RawDocument, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return nil, errors.New(
			"evidence snapshot store is not initialized",
		)
	}
	if query.AsOf.IsZero() {
		return nil, errors.New(
			"evidence snapshot query as_of is required",
		)
	}
	if !query.TimeRange.Start.IsZero() &&
		!query.TimeRange.End.IsZero() &&
		query.TimeRange.Start.After(
			query.TimeRange.End,
		) {
		return nil, errors.New(
			"evidence snapshot query time range start is after end",
		)
	}

	root := filepath.Join(
		s.root,
		evidenceSnapshotDirectory,
	)
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []RawDocument{}, nil
		}
		return nil, fmt.Errorf(
			"stat evidence snapshot store: %w",
			err,
		)
	}

	selected := make(
		map[string]EvidenceSnapshot,
	)
	err := filepath.WalkDir(
		root,
		func(
			path string,
			entry fs.DirEntry,
			walkErr error,
		) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() ||
				filepath.Ext(entry.Name()) != ".json" {
				return nil
			}

			snapshot, err := readEvidenceSnapshot(
				path,
			)
			if err != nil {
				return err
			}
			if !snapshotMatchesQuery(
				snapshot,
				query,
			) {
				return nil
			}

			key := snapshot.LogicalDocumentID +
				"\x00" +
				snapshot.VersionID
			current, exists := selected[key]
			if !exists ||
				snapshotObservationTime(snapshot).
					Before(
						snapshotObservationTime(current),
					) {
				selected[key] = snapshot
			}
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"query evidence snapshots: %w",
			err,
		)
	}

	snapshots := make(
		[]EvidenceSnapshot,
		0,
		len(selected),
	)
	for _, snapshot := range selected {
		snapshots = append(
			snapshots,
			snapshot,
		)
	}
	sort.SliceStable(
		snapshots,
		func(i int, j int) bool {
			leftAvailable := effectiveAvailableAt(
				snapshots[i].Document,
			)
			rightAvailable := effectiveAvailableAt(
				snapshots[j].Document,
			)

			switch {
			case leftAvailable != nil &&
				rightAvailable != nil &&
				leftAvailable.Before(*rightAvailable):
				return true
			case leftAvailable != nil &&
				rightAvailable != nil &&
				rightAvailable.Before(*leftAvailable):
				return false
			case snapshots[i].LogicalDocumentID !=
				snapshots[j].LogicalDocumentID:
				return snapshots[i].LogicalDocumentID <
					snapshots[j].LogicalDocumentID
			default:
				return snapshots[i].VersionID <
					snapshots[j].VersionID
			}
		},
	)

	documents := make(
		[]RawDocument,
		0,
		len(snapshots),
	)
	for _, snapshot := range snapshots {
		documents = append(
			documents,
			snapshot.Document,
		)
	}
	return documents, nil
}

func evidenceSnapshotFromRaw(
	document RawDocument,
) (EvidenceSnapshot, error) {
	document.ID = strings.TrimSpace(document.ID)
	if document.ID == "" {
		return EvidenceSnapshot{}, errors.New(
			"evidence snapshot raw document id is required",
		)
	}
	if document.RetrievedAt.IsZero() {
		return EvidenceSnapshot{}, fmt.Errorf(
			"evidence snapshot raw document %q retrieved_at is required",
			document.ID,
		)
	}

	logicalKey := canonicalLogicalDocumentKey(
		document,
	)
	if logicalKey == "" {
		return EvidenceSnapshot{}, fmt.Errorf(
			"evidence snapshot raw document %q has no logical identity",
			document.ID,
		)
	}
	logicalID := canonicalDocumentID(
		logicalKey,
	)
	version := documentVersionFromRaw(
		document,
		logicalID,
		logicalKey,
		effectiveAvailableAt(document),
	)

	snapshotID := shortID(
		"snapshot",
		logicalID,
		version.ID,
		document.ID,
		document.RetrievedAt.UTC().
			Format(time.RFC3339Nano),
	)

	return EvidenceSnapshot{
		SchemaVersion:     evidenceSnapshotSchemaVersion,
		ID:                snapshotID,
		LogicalDocumentID: logicalID,
		VersionID:         version.ID,
		Document:          document,
	}, nil
}

func (s *FileEvidenceSnapshotStore) writeSnapshot(
	ctx context.Context,
	snapshot EvidenceSnapshot,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dir := filepath.Join(
		s.root,
		evidenceSnapshotDirectory,
		snapshot.LogicalDocumentID,
		snapshot.VersionID,
	)
	if err := os.MkdirAll(
		dir,
		0o755,
	); err != nil {
		return fmt.Errorf(
			"create evidence snapshot directory: %w",
			err,
		)
	}

	target := filepath.Join(
		dir,
		snapshot.ID+".json",
	)
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"stat evidence snapshot %q: %w",
			snapshot.ID,
			err,
		)
	}

	data, err := json.MarshalIndent(
		snapshot,
		"",
		"  ",
	)
	if err != nil {
		return fmt.Errorf(
			"encode evidence snapshot %q: %w",
			snapshot.ID,
			err,
		)
	}
	data = append(data, '\n')

	temp, err := os.CreateTemp(
		dir,
		".snapshot-*.tmp",
	)
	if err != nil {
		return fmt.Errorf(
			"create evidence snapshot temp file: %w",
			err,
		)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf(
			"chmod evidence snapshot temp file: %w",
			err,
		)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf(
			"write evidence snapshot temp file: %w",
			err,
		)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf(
			"sync evidence snapshot temp file: %w",
			err,
		)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf(
			"close evidence snapshot temp file: %w",
			err,
		)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"stat evidence snapshot %q before rename: %w",
			snapshot.ID,
			err,
		)
	}

	if err := os.Link(
		tempName,
		target,
	); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		if _, statErr := os.Stat(target);
			statErr == nil {
			return nil
		}
		return fmt.Errorf(
			"commit immutable evidence snapshot %q: %w",
			snapshot.ID,
			err,
		)
	}

	return nil
}

func readEvidenceSnapshot(
	path string,
) (EvidenceSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EvidenceSnapshot{}, fmt.Errorf(
			"read evidence snapshot %q: %w",
			path,
			err,
		)
	}

	var snapshot EvidenceSnapshot
	if err := json.Unmarshal(
		data,
		&snapshot,
	); err != nil {
		return EvidenceSnapshot{}, fmt.Errorf(
			"decode evidence snapshot %q: %w",
			path,
			err,
		)
	}

	if snapshot.SchemaVersion !=
		evidenceSnapshotSchemaVersion {
		return EvidenceSnapshot{}, fmt.Errorf(
			"evidence snapshot %q has unsupported schema version %d",
			path,
			snapshot.SchemaVersion,
		)
	}

	if strings.TrimSpace(snapshot.ID) == "" ||
		strings.TrimSpace(
			snapshot.LogicalDocumentID,
		) == "" ||
		strings.TrimSpace(snapshot.VersionID) == "" ||
		strings.TrimSpace(snapshot.Document.ID) == "" {
		return EvidenceSnapshot{}, fmt.Errorf(
			"evidence snapshot %q is malformed",
			path,
		)
	}

	return snapshot, nil
}

func snapshotMatchesQuery(
	snapshot EvidenceSnapshot,
	query EvidenceSnapshotQuery,
) bool {
	document := snapshot.Document
	availableAt := effectiveAvailableAt(
		document,
	)
	if availableAt == nil ||
		availableAt.After(query.AsOf) {
		return false
	}
	if document.PublishedAt != nil &&
		document.PublishedAt.After(query.AsOf) {
		return false
	}
	if document.UpdatedAt != nil &&
		document.UpdatedAt.After(query.AsOf) {
		return false
	}

	eventTime := availableAt
	if document.PublishedAt != nil {
		eventTime = document.PublishedAt
	}
	if !query.TimeRange.Start.IsZero() &&
		eventTime.Before(query.TimeRange.Start) {
		return false
	}
	if !query.TimeRange.End.IsZero() &&
		eventTime.After(query.TimeRange.End) {
		return false
	}

	if !matchesSourceType(
		document.SourceType,
		query.SourceTypes,
	) {
		return false
	}
	if !matchesAuthority(
		document.Authority,
		query.Authorities,
	) {
		return false
	}
	if !matchesEvidenceKind(
		document.EvidenceKind,
		query.EvidenceKinds,
	) {
		return false
	}
	if !snapshotMatchesSymbol(
		document,
		query.Symbol,
	) {
		return false
	}

	return true
}

func matchesSourceType(
	value SourceType,
	allowed []SourceType,
) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func matchesAuthority(
	value EvidenceAuthority,
	allowed []EvidenceAuthority,
) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func matchesEvidenceKind(
	value EvidenceKind,
	allowed []EvidenceKind,
) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func snapshotMatchesSymbol(
	document RawDocument,
	symbol string,
) bool {
	symbol = strings.ToUpper(
		strings.TrimSpace(symbol),
	)
	if symbol == "" {
		return true
	}

	candidates := []string{
		snapshotMetadataString(
			document.RawMetadata,
			"symbol",
		),
		snapshotMetadataString(
			document.RawMetadata,
			"instrument_id",
		),
	}
	baseSymbol := snapshotBaseSymbol(
		symbol,
	)
	for _, candidate := range candidates {
		candidate = strings.ToUpper(
			strings.TrimSpace(candidate),
		)
		if candidate == "" {
			continue
		}
		if candidate == symbol ||
			snapshotBaseSymbol(candidate) ==
				baseSymbol {
			return true
		}
	}
	return false
}

func snapshotMetadataString(
	metadata map[string]any,
	key string,
) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func snapshotBaseSymbol(
	symbol string,
) string {
	symbol = strings.ToUpper(
		strings.TrimSpace(symbol),
	)
	switch {
	case strings.HasSuffix(symbol, ".SH"),
		strings.HasSuffix(symbol, ".SZ"),
		strings.HasSuffix(symbol, ".BJ"):
		return symbol[:len(symbol)-3]
	case len(symbol) == 8 &&
		(symbol[:2] == "SH" ||
			symbol[:2] == "SZ" ||
			symbol[:2] == "BJ"):
		return symbol[2:]
	default:
		return symbol
	}
}

func snapshotObservationTime(
	snapshot EvidenceSnapshot,
) time.Time {
	if snapshot.Document.FirstSeenAt != nil {
		return *snapshot.Document.FirstSeenAt
	}
	return snapshot.Document.RetrievedAt
}
