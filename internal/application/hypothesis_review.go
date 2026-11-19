package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type HypothesisReviewDraft struct {
	ObservationRunID string               `json:"observation_run_id"`
	Category         string               `json:"category"`
	Reason           string               `json:"reason"`
	Supporting       []HypothesisEvidence `json:"supporting_evidence"`
	Counterexamples  []HypothesisEvidence `json:"counterexamples"`
}

type HypothesisReview struct {
	ID           string    `json:"review_id"`
	HypothesisID string    `json:"hypothesis_id"`
	SourceRunID  string    `json:"source_run_id"`
	CreatedAt    time.Time `json:"created_at"`
	HypothesisReviewDraft
}

var ErrInvalidHypothesisReview = errors.New("invalid hypothesis review")
var ErrReviewObservationMissing = errors.New("review requires a stored observation for this hypothesis")

func (draft *HypothesisReviewDraft) normalize() error {
	switch draft.Category {
	case "DATA_MISSING", "CITATION_ERROR", "REASONING_BIAS", "CONTEXT_CHANGED", "HINDSIGHT_BIAS":
	default:
		return ErrInvalidHypothesisReview
	}
	draft.Reason = strings.TrimSpace(draft.Reason)
	if !validRunID(draft.ObservationRunID) || draft.Reason == "" || len(draft.Reason) > 2048 || len(draft.Supporting) == 0 {
		return ErrInvalidHypothesisReview
	}
	var err error
	draft.Supporting, err = normalizeEvidence(draft.Supporting)
	if err != nil {
		return ErrInvalidHypothesisReview
	}
	draft.Counterexamples, err = normalizeEvidence(draft.Counterexamples)
	if err != nil {
		return ErrInvalidHypothesisReview
	}
	return nil
}

func reviewID(review HypothesisReview) (string, error) {
	review.ID = ""
	review.CreatedAt = time.Time{}
	raw, err := json.Marshal(review)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16]), nil
}

func (s *FileHypothesisStore) AddReview(runID, hypothesisID string, draft HypothesisReviewDraft, now time.Time) (HypothesisReview, bool, error) {
	if err := draft.normalize(); err != nil {
		return HypothesisReview{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.find(runID, hypothesisID)
	if err != nil {
		return HypothesisReview{}, false, err
	}
	observations, err := s.readObservations(item.ID)
	if err != nil {
		return HypothesisReview{}, false, err
	}
	for _, observation := range observations {
		if observation.RunID == draft.ObservationRunID {
			review := HypothesisReview{
				HypothesisID: item.ID, SourceRunID: item.RunID, CreatedAt: now.UTC(), HypothesisReviewDraft: draft,
			}
			return s.saveReview(review)
		}
	}
	return HypothesisReview{}, false, ErrReviewObservationMissing
}

func (s *FileHypothesisStore) saveReview(review HypothesisReview) (HypothesisReview, bool, error) {
	id, err := reviewID(review)
	if err != nil {
		return HypothesisReview{}, false, err
	}
	review.ID = id
	records, err := s.readReviews(review.SourceRunID, review.HypothesisID)
	if err != nil {
		return HypothesisReview{}, false, err
	}
	for _, previous := range records {
		if previous.ID == review.ID {
			return previous, false, nil
		}
	}
	if len(records) >= maxRunHypotheses {
		return HypothesisReview{}, false, ErrHypothesisCapacity
	}
	if err := writeJSONFile(s.reviewPath(review.HypothesisID), append(records, review)); err != nil {
		return HypothesisReview{}, false, err
	}
	return review, true, nil
}

func (s *FileHypothesisStore) Reviews(runID, hypothesisID string) ([]HypothesisReview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.find(runID, hypothesisID)
	if err != nil {
		return nil, err
	}
	return s.readReviews(item.RunID, item.ID)
}

func (s *FileHypothesisStore) reviewPath(id string) string {
	return filepath.Join(s.dir, "reviews-"+id+".json")
}

func (s *FileHypothesisStore) readReviews(runID, hypothesisID string) ([]HypothesisReview, error) {
	records := []HypothesisReview{}
	raw, err := os.ReadFile(s.reviewPath(hypothesisID))
	if errors.Is(err, os.ErrNotExist) {
		return records, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	if records == nil || len(records) > maxRunHypotheses {
		return nil, errors.New("invalid persisted reviews")
	}
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		id, err := reviewID(record)
		if err != nil || id != record.ID || record.HypothesisID != hypothesisID || record.SourceRunID != runID ||
			record.CreatedAt.IsZero() || seen[id] {
			return nil, errors.New("invalid persisted review")
		}
		seen[id] = true
	}
	return records, nil
}
