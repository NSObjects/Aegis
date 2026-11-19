package application

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"
)

// priorHypotheses is called with the hypothesis store lock held.
func (s *FileHypothesisStore) priorHypotheses(ctx context.Context, symbol string, cutoff time.Time) ([]ResearchHypothesis, error) {
	files, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	items := make([]ResearchHypothesis, 0)
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		runID := strings.TrimSuffix(file.Name(), ".json")
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") || !validRunID(runID) {
			continue
		}
		records, err := s.read(runID)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			if record.Symbol == symbol && record.AsOf.Before(cutoff) && !record.CreatedAt.After(cutoff) {
				items = append(items, record)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].AsOf.After(items[j].AsOf) })
	return items, nil
}

// ObserveAutomaticRun reuses the evidence rules of manual observation. The
// original reports and curated memory are untouched by an automatic pass.
func (s *FileHypothesisStore) ObserveAutomaticRun(ctx context.Context, run RunRecord) error {
	if run.State != RunCompleted || !ValidWatchSymbol(run.Symbol) || run.RequestedAsOf.IsZero() {
		return ErrObservationSource
	}
	s.mu.Lock()
	items, err := s.priorHypotheses(ctx, run.Symbol, run.RequestedAsOf)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A scheduled pass follows active structured criteria only. Archived
		// observations after expiry remain available through explicit linking.
		if run.RequestedAsOf.After(item.ExpiresAt) || (item.CloseRule == nil && item.DocumentRule == nil) {
			continue
		}
		if item.CloseRule != nil {
			closeAt, err := hypothesisMarketClose(item.CloseRule.TradeDate)
			if err != nil || run.RequestedAsOf.Before(closeAt) {
				continue
			}
		}
		if _, _, err := s.Observe(item.RunID, item.ID, run); err != nil && !errors.Is(err, ErrHypothesisCapacity) {
			return err
		}
	}
	return nil
}

type ConversationMemoryArgs struct {
	Symbol string `json:"symbol" jsonschema:"required,description=A-share symbol such as 600519.SH"`
	Query  string `json:"q,omitempty" jsonschema:"description=Optional exact thesis keyword"`
}

type ConversationHypothesis struct {
	ID          string                 `json:"hypothesis_id"`
	SourceRunID string                 `json:"source_run_id"`
	Thesis      string                 `json:"thesis"`
	AsOf        time.Time              `json:"as_of"`
	ExpiresAt   time.Time              `json:"expires_at"`
	Latest      *HypothesisObservation `json:"latest_observation,omitempty"`
}

type ConversationMemorySample struct {
	Memory      ResearchMemory        `json:"memory"`
	Observation HypothesisObservation `json:"observation"`
	Review      *HypothesisReview     `json:"review,omitempty"`
}

type ConversationMemoryResult struct {
	Symbol     string                     `json:"symbol"`
	AsOf       time.Time                  `json:"lookup_as_of"`
	Hypotheses []ConversationHypothesis   `json:"hypotheses"`
	Samples    []ConversationMemorySample `json:"memory_samples"`
}

func (s *FileHypothesisStore) ConversationMemory(ctx context.Context, args ConversationMemoryArgs, cutoff time.Time) (ConversationMemoryResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(args.Symbol))
	query := strings.ToLower(strings.TrimSpace(args.Query))
	if !ValidWatchSymbol(symbol) || len(query) > 512 || cutoff.IsZero() {
		return ConversationMemoryResult{}, ErrInvalidMemory
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.priorHypotheses(ctx, symbol, cutoff)
	if err != nil {
		return ConversationMemoryResult{}, err
	}
	memory, err := s.readMemory()
	if err != nil {
		return ConversationMemoryResult{}, err
	}
	sort.SliceStable(memory, func(i, j int) bool { return memory[i].CreatedAt.After(memory[j].CreatedAt) })
	result := ConversationMemoryResult{
		Symbol: symbol, AsOf: cutoff.UTC(),
		Hypotheses: []ConversationHypothesis{}, Samples: []ConversationMemorySample{},
	}
	// A revoked archival sample never contributes its review or sample metadata
	// to chat. The independent hypothesis and observation history may be read.
	for _, item := range items {
		if !strings.Contains(strings.ToLower(item.Statement), query) {
			continue
		}
		observations, err := s.readObservations(item.ID)
		if err != nil {
			return ConversationMemoryResult{}, err
		}
		eligible := make(map[string]HypothesisObservation, len(observations))
		hypothesis := ConversationHypothesis{
			ID: item.ID, SourceRunID: item.RunID, Thesis: item.Statement, AsOf: item.AsOf, ExpiresAt: item.ExpiresAt,
		}
		for _, obs := range observations {
			if obs.AsOf.After(cutoff) {
				continue
			}
			eligible[obs.RunID] = obs
			if hypothesis.Latest == nil || obs.AsOf.After(hypothesis.Latest.AsOf) {
				hypothesis.Latest = &obs
			}
		}
		if len(result.Hypotheses) < 5 {
			result.Hypotheses = append(result.Hypotheses, hypothesis)
		}
		for _, entry := range memory {
			observation, found := eligible[entry.ObservationRunID]
			if len(result.Samples) == 5 {
				break
			}
			if !found || entry.HypothesisID != item.ID || entry.RevokedAt != nil || entry.CreatedAt.After(cutoff) {
				continue
			}
			feedback := make([]ResearchFeedback, 0, len(entry.Feedback))
			for _, item := range entry.Feedback {
				if !item.RecordedAt.After(cutoff) {
					feedback = append(feedback, item)
				}
			}
			entry.Feedback = feedback
			sample := ConversationMemorySample{Memory: entry, Observation: observation}
			if entry.ReviewID != "" {
				reviews, err := s.readReviews(item.RunID, item.ID)
				if err != nil {
					return ConversationMemoryResult{}, err
				}
				for _, review := range reviews {
					if review.ID == entry.ReviewID && !review.CreatedAt.After(cutoff) {
						sample.Review = &review
						break
					}
				}
				if sample.Review == nil {
					continue
				}
			}
			result.Samples = append(result.Samples, sample)
		}
		if len(result.Hypotheses) == 5 && len(result.Samples) == 5 {
			break
		}
	}
	return result, nil
}
