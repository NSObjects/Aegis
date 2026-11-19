package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxRunHypotheses = 50

var (
	ErrInvalidHypothesis  = errors.New("invalid research hypothesis")
	ErrHypothesisSource   = errors.New("hypothesis requires a completed research report")
	ErrHypothesisCapacity = errors.New("research hypothesis capacity reached")
)

type HypothesisEvidence struct {
	Reference string `json:"reference"`
	Detail    string `json:"detail"`
}

// HypothesisDraft is the operator's assertion and the evidence they cited.
type HypothesisDraft struct {
	Statement        string                  `json:"statement"`
	ExpiresAt        time.Time               `json:"expires_at"`
	Supporting       []HypothesisEvidence    `json:"supporting_evidence"`
	Contrary         []HypothesisEvidence    `json:"contrary_evidence"`
	Invalidation     string                  `json:"invalidation"`
	VerificationRule string                  `json:"verification_rule"`
	CloseRule        *HypothesisCloseRule    `json:"close_rule,omitempty"`
	DocumentRule     *HypothesisDocumentRule `json:"document_rule,omitempty"`
}

// ResearchHypothesis is an immutable annotation of a particular source report.
type ResearchHypothesis struct {
	ID             string    `json:"hypothesis_id"`
	RunID          string    `json:"run_id"`
	Symbol         string    `json:"symbol"`
	AsOf           time.Time `json:"as_of"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	CreatedAt      time.Time `json:"created_at"`
	HypothesisDraft
}

type FileHypothesisStore struct {
	dir string
	mu  sync.Mutex
}

func NewFileHypothesisStore(dir string) (*FileHypothesisStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("hypothesis directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("hypothesis directory must be private")
	}
	return &FileHypothesisStore{dir: dir}, nil
}

func normalizeEvidence(items []HypothesisEvidence) ([]HypothesisEvidence, error) {
	if len(items) > 8 {
		return nil, fmt.Errorf("%w: at most eight references per side", ErrInvalidHypothesis)
	}
	result := make([]HypothesisEvidence, 0, len(items))
	for _, item := range items {
		item.Reference = strings.TrimSpace(item.Reference)
		item.Detail = strings.TrimSpace(item.Detail)
		if item.Reference == "" || len(item.Reference) > 512 || item.Detail == "" || len(item.Detail) > 512 {
			return nil, fmt.Errorf("%w: each evidence item requires a reference and detail", ErrInvalidHypothesis)
		}
		result = append(result, item)
	}
	return result, nil
}

func (h *ResearchHypothesis) normalize() error {
	h.Statement = strings.TrimSpace(h.Statement)
	h.Invalidation = strings.TrimSpace(h.Invalidation)
	h.VerificationRule = strings.TrimSpace(h.VerificationRule)
	for _, value := range []string{h.Statement, h.Invalidation, h.VerificationRule} {
		if value == "" || len(value) > 1024 {
			return fmt.Errorf("%w: statement, invalidation and verification rule are required", ErrInvalidHypothesis)
		}
	}
	if !h.ExpiresAt.After(h.AsOf) {
		return fmt.Errorf("%w: expiration must be later than the source cutoff", ErrInvalidHypothesis)
	}
	var err error
	if h.Supporting, err = normalizeEvidence(h.Supporting); err != nil {
		return err
	}
	if h.Contrary, err = normalizeEvidence(h.Contrary); err != nil {
		return err
	}
	if rule := h.CloseRule; rule != nil {
		closeAt, err := hypothesisMarketClose(rule.TradeDate)
		if err != nil || !closeAt.After(h.AsOf) || closeAt.After(h.ExpiresAt) ||
			(rule.Operator != "GTE" && rule.Operator != "LTE") || !validHypothesisPrice(rule.Threshold) {
			return fmt.Errorf("%w: invalid closing price rule", ErrInvalidHypothesis)
		}
	}
	if rule := h.DocumentRule; rule != nil {
		if h.CloseRule != nil || !validIndexID(rule.DocumentID) ||
			(rule.OnMatch != "SUPPORTED" && rule.OnMatch != "REFUTED") {
			return fmt.Errorf("%w: invalid event document rule", ErrInvalidHypothesis)
		}
	}
	h.ExpiresAt = h.ExpiresAt.UTC()
	return nil
}

func hypothesisID(h ResearchHypothesis) (string, error) {
	h.ID = ""
	h.CreatedAt = time.Time{}
	raw, err := json.Marshal(h)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16]), nil
}

func (s *FileHypothesisStore) Add(run RunRecord, draft HypothesisDraft, now time.Time) (ResearchHypothesis, bool, error) {
	digest := artifactDigest(run.Artifact)
	if run.State != RunCompleted || !validRunID(run.ID) || !ValidWatchSymbol(run.Symbol) ||
		run.RequestedAsOf.IsZero() || len(run.Artifact) == 0 || digest == "" ||
		(run.ArtifactSHA256 != "" && run.ArtifactSHA256 != digest) {
		return ResearchHypothesis{}, false, ErrHypothesisSource
	}
	item := ResearchHypothesis{
		RunID: run.ID, Symbol: run.Symbol, AsOf: run.RequestedAsOf.UTC(),
		ArtifactSHA256: digest, CreatedAt: now.UTC(), HypothesisDraft: draft,
	}
	if err := item.normalize(); err != nil {
		return ResearchHypothesis{}, false, err
	}
	id, err := hypothesisID(item)
	if err != nil {
		return ResearchHypothesis{}, false, fmt.Errorf("%w: invalid time", ErrInvalidHypothesis)
	}
	item.ID = id

	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.read(run.ID)
	if err != nil {
		return ResearchHypothesis{}, false, err
	}
	for _, previous := range items {
		if previous.ID == item.ID {
			return previous, false, nil
		}
	}
	if len(items) >= maxRunHypotheses {
		return ResearchHypothesis{}, false, ErrHypothesisCapacity
	}
	if err := writeJSONFile(s.path(run.ID), append(items, item)); err != nil {
		return ResearchHypothesis{}, false, err
	}
	return item, true, nil
}

func (s *FileHypothesisStore) List(runID string) ([]ResearchHypothesis, error) {
	if !validRunID(runID) {
		return nil, ErrHypothesisSource
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(runID)
}

func (s *FileHypothesisStore) path(runID string) string {
	return filepath.Join(s.dir, runID+".json")
}

func (s *FileHypothesisStore) read(runID string) ([]ResearchHypothesis, error) {
	items := make([]ResearchHypothesis, 0)
	raw, err := os.ReadFile(s.path(runID))
	if errors.Is(err, os.ErrNotExist) {
		return items, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	if items == nil || len(items) > maxRunHypotheses {
		return nil, errors.New("invalid persisted hypotheses")
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		id, err := hypothesisID(item)
		if err != nil || item.RunID != runID || id != item.ID || item.CreatedAt.IsZero() || seen[id] {
			return nil, errors.New("invalid persisted hypothesis")
		}
		seen[id] = true
	}
	return items, nil
}
