package application

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const maxRunRecordBytes int64 = 64 << 20

// FileRunStore is a single-process, durable implementation for early Web
// development. It atomically replaces one run snapshot per JSON file.
// It must not be shared by multiple processes; replace it with transactional
// storage before adding multiple API workers or distributed scheduling.
type FileRunStore struct {
	dir string
	mu  sync.Mutex
}

func NewFileRunStore(dir string) (*FileRunStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("run store directory is required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create run directory: %w", err)
	}
	stat, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, errors.New("run store path is not a directory")
	}
	// Outputs can contain confidential research or caller-supplied portfolio data.
	// Never store them under a world-readable run directory.
	if stat.Mode().Perm()&0077 != 0 {
		return nil, errors.New("run store directory must be private (0700)")
	}
	return &FileRunStore{dir: dir}, nil
}

func validRunID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func checkRunRecord(record RunRecord) error {
	if !validRunID(record.ID) {
		return errors.New("invalid research run ID")
	}
	if (record.IdempotencyDigest == "") != (record.RequestFingerprint == "") {
		return errors.New("incomplete research idempotency metadata")
	}
	if record.IdempotencyDigest != "" {
		for _, value := range []string{record.IdempotencyDigest, record.RequestFingerprint} {
			decoded, err := hex.DecodeString(value)
			if err != nil || len(value) != 64 || len(decoded) != 32 || value != strings.ToLower(value) {
				return errors.New("invalid research idempotency digest")
			}
		}
	}
	if record.RerunOf != "" && (!validRunID(record.RerunOf) || record.RerunOf == record.ID) {
		return errors.New("invalid rerun provenance")
	}
	if record.Mode != ModeDecision && record.Mode != ModePortfolio {
		return errors.New("invalid research run mode")
	}
	if strings.TrimSpace(record.Symbol) == "" || record.RequestedAsOf.IsZero() || record.CreatedAt.IsZero() {
		return errors.New("incomplete research run metadata")
	}
	switch record.State {
	case RunPending, RunRunning, RunCompleted, RunFailed, RunCancelled:
	default:
		return errors.New("invalid research run state")
	}
	if record.State == RunCompleted {
		if record.FinishedAt == nil || len(record.Artifact) == 0 || !json.Valid(record.Artifact) || string(record.Artifact) == "null" || record.FailureCode != "" {
			return errors.New("completed research must contain valid artifact")
		}
		// Legacy completed runs (version=0 with no hash) remain readable.
		// New completed runs must carry a verified checksum; never silently
		// accept a corrupted or unknown-version artifact after restart.
		if record.ArtifactVersion == 0 {
			if record.ArtifactSHA256 != "" { return errors.New("legacy artifact must not declare a checksum without a version") }
		} else {
			if record.ArtifactVersion != 1 { return errors.New("unsupported research artifact version") }
			if len(record.ArtifactSHA256) != 64 || record.ArtifactSHA256 != artifactDigest(record.Artifact) {
				return errors.New("research artifact checksum mismatch")
			}
		}
	} else if len(record.Artifact) != 0 || record.ArtifactVersion != 0 || record.ArtifactSHA256 != "" {
		return errors.New("unfinished research must not expose an artifact or checksum")
	}
	if record.State == RunFailed || record.State == RunCancelled {
		if record.FinishedAt == nil {
			return errors.New("terminal run requires completion time")
		}
	}
	for i, ev := range record.Events {
		if ev.Seq != uint64(i+1) || ev.Stage == "" || ev.Status == "" || ev.At.IsZero() || ev.DurationMS < 0 {
			return errors.New("invalid research event sequence")
		}
	}
	if record.State == RunPending && (record.StartedAt != nil || record.FinishedAt != nil) {
		return errors.New("pending run cannot have execution times")
	}
	return nil
}
func (s *FileRunStore) Save(ctx context.Context, record RunRecord) error {
	if s == nil {
		return errors.New("run store is nil")
	}
	if ctx == nil {
		return errors.New("save context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkRunRecord(record); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode research run: %w", err)
	}
	if int64(len(payload)) > maxRunRecordBytes {
		return errors.New("research run snapshot exceeds size limit")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tmp, err := os.CreateTemp(s.dir, ".run-*.tmp")
	if err != nil {
		return fmt.Errorf("create run temp file: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	defer tmp.Close()
	if err = tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err = tmp.Write(payload); err != nil {
		return fmt.Errorf("write run snapshot: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync run snapshot: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(name, s.filePath(record.ID)); err != nil {
		return fmt.Errorf("replace run snapshot: %w", err)
	}
	// Persist the rename for crash recovery on filesystems supporting dir fsync.
	d, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return fmt.Errorf("sync run directory: %w", err)
	}
	return nil
}
func (s *FileRunStore) filePath(id string) string { return filepath.Join(s.dir, id+".json") }
func (s *FileRunStore) Get(ctx context.Context, id string) (RunRecord, error) {
	if s == nil {
		return RunRecord{}, errors.New("run store is nil")
	}
	if ctx == nil {
		return RunRecord{}, errors.New("get context is nil")
	}
	if err := ctx.Err(); err != nil {
		return RunRecord{}, err
	}
	if !validRunID(id) {
		return RunRecord{}, errors.New("invalid research run ID")
	}
	f, err := os.Open(s.filePath(id))
	if errors.Is(err, os.ErrNotExist) {
		return RunRecord{}, ErrRunNotFound
	}
	if err != nil {
		return RunRecord{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return RunRecord{}, err
	}
	if info.Size() > maxRunRecordBytes {
		return RunRecord{}, errors.New("research run file exceeds limit")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxRunRecordBytes+1))
	if err != nil {
		return RunRecord{}, err
	}
	if int64(len(raw)) > maxRunRecordBytes {
		return RunRecord{}, errors.New("research run file exceeds limit")
	}
	var result RunRecord
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&result); err != nil {
		return RunRecord{}, fmt.Errorf("decode research run: %w", err)
	}
	var trailing any
	if err = dec.Decode(&trailing); err != io.EOF {
		return RunRecord{}, errors.New("research run file contains trailing JSON")
	}
	if result.ID != id {
		return RunRecord{}, errors.New("research run file ID mismatch")
	}
	if err := checkRunRecord(result); err != nil {
		return RunRecord{}, err
	}
	return result, nil
}
func (s *FileRunStore) List(ctx context.Context) ([]RunRecord, error) {
	if s == nil {
		return nil, errors.New("run store is nil")
	}
	if ctx == nil {
		return nil, errors.New("list context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	records := make([]RunRecord, 0, len(entries))
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		record, err := s.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("read research run %s: %w", e.Name(), err)
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records, nil
}
