package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxResearchMemory = 1000

var (
	ErrInvalidMemory  = errors.New("invalid research memory request")
	ErrMemorySource   = errors.New("research memory requires a stored observation and matching optional review")
	ErrMemoryNotFound = errors.New("research memory not found")
	ErrMemoryCapacity = errors.New("research memory capacity reached")
	ErrMemoryRevoked  = errors.New("research memory was revoked")
)

type MemorySource struct {
	SourceRunID      string `json:"source_run_id"`
	HypothesisID     string `json:"hypothesis_id"`
	ObservationRunID string `json:"observation_run_id"`
	ReviewID         string `json:"review_id,omitempty"`
	MarketPhase      string `json:"market_phase,omitempty"`
}

type ResearchMemory struct {
	ID        string     `json:"memory_id"`
	Symbol    string     `json:"symbol"`
	Thesis    string     `json:"thesis"`
	Outcome   string     `json:"outcome"`
	Category  string     `json:"category,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	MemorySource
	Feedback []ResearchFeedback `json:"feedback,omitempty"`
}

type MemoryFilter struct {
	Symbol, Query, MarketPhase, Category, Outcome string
	Offset                                        int
}

type MemoryResults struct {
	Memories []ResearchMemory `json:"memories"`
	Total    int              `json:"total"`
	HasMore  bool             `json:"has_more"`
}

func memoryID(source MemorySource) (string, error) {
	raw, err := json.Marshal(source)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16]), nil
}

func (s *FileHypothesisStore) AddMemory(source MemorySource, now time.Time) (ResearchMemory, bool, error) {
	source.MarketPhase = strings.TrimSpace(source.MarketPhase)
	if !validRunID(source.SourceRunID) || !validRunID(source.HypothesisID) ||
		!validRunID(source.ObservationRunID) || (source.ReviewID != "" && !validRunID(source.ReviewID)) ||
		len(source.MarketPhase) > 128 || now.IsZero() {
		return ResearchMemory{}, false, ErrInvalidMemory
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.find(source.SourceRunID, source.HypothesisID)
	if err != nil {
		return ResearchMemory{}, false, err
	}
	observations, err := s.readObservations(item.ID)
	if err != nil {
		return ResearchMemory{}, false, err
	}
	entry := ResearchMemory{MemorySource: source, Symbol: item.Symbol, Thesis: item.Statement, CreatedAt: now.UTC()}
	for _, observation := range observations {
		if observation.RunID == source.ObservationRunID {
			entry.Outcome = observation.Status
			break
		}
	}
	if entry.Outcome == "" {
		return ResearchMemory{}, false, ErrMemorySource
	}
	if source.ReviewID != "" {
		reviews, err := s.readReviews(item.RunID, item.ID)
		if err != nil {
			return ResearchMemory{}, false, err
		}
		for _, review := range reviews {
			if review.ID == source.ReviewID && review.ObservationRunID == source.ObservationRunID {
				entry.Category = review.Category
				break
			}
		}
		if entry.Category == "" {
			return ResearchMemory{}, false, ErrMemorySource
		}
	}
	return s.addMemory(entry)
}

func (s *FileHypothesisStore) addMemory(entry ResearchMemory) (ResearchMemory, bool, error) {
	id, err := memoryID(entry.MemorySource)
	if err != nil {
		return ResearchMemory{}, false, err
	}
	entry.ID = id
	records, err := s.readMemory()
	if err != nil {
		return ResearchMemory{}, false, err
	}
	for _, existing := range records {
		if existing.ID == id {
			if existing.RevokedAt != nil {
				return ResearchMemory{}, false, ErrMemoryRevoked
			}
			return existing, false, nil
		}
	}
	if len(records) >= maxResearchMemory {
		return ResearchMemory{}, false, ErrMemoryCapacity
	}
	if err := writeJSONFile(s.memoryPath(), append(records, entry)); err != nil {
		return ResearchMemory{}, false, err
	}
	return entry, true, nil
}

func (s *FileHypothesisStore) RevokeMemory(id string, now time.Time) (ResearchMemory, error) {
	if !validRunID(id) || now.IsZero() {
		return ResearchMemory{}, ErrInvalidMemory
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.readMemory()
	if err != nil {
		return ResearchMemory{}, err
	}
	for i, entry := range records {
		if entry.ID != id {
			continue
		}
		if entry.RevokedAt != nil {
			return entry, nil
		}
		at := now.UTC()
		entry.RevokedAt = &at
		records[i] = entry
		if err := writeJSONFile(s.memoryPath(), records); err != nil {
			return ResearchMemory{}, err
		}
		return entry, nil
	}
	return ResearchMemory{}, ErrMemoryNotFound
}

func (s *FileHypothesisStore) SearchMemory(filter MemoryFilter) (MemoryResults, error) {
	filter.Symbol = strings.ToUpper(strings.TrimSpace(filter.Symbol))
	filter.Query = strings.ToLower(strings.TrimSpace(filter.Query))
	filter.MarketPhase = strings.TrimSpace(filter.MarketPhase)
	if (filter.Symbol != "" && !ValidWatchSymbol(filter.Symbol)) || len(filter.Query) > 512 ||
		len(filter.MarketPhase) > 128 || filter.Offset < 0 || filter.Offset > maxResearchMemory {
		return MemoryResults{}, ErrInvalidMemory
	}
	switch filter.Outcome {
	case "", "SUPPORTED", "REFUTED", "INCONCLUSIVE", "EXPIRED":
	default:
		return MemoryResults{}, ErrInvalidMemory
	}
	switch filter.Category {
	case "", "DATA_MISSING", "CITATION_ERROR", "REASONING_BIAS", "CONTEXT_CHANGED", "HINDSIGHT_BIAS":
	default:
		return MemoryResults{}, ErrInvalidMemory
	}

	s.mu.Lock()
	records, err := s.readMemory()
	s.mu.Unlock()
	if err != nil {
		return MemoryResults{}, err
	}
	matches := make([]ResearchMemory, 0)
	for _, entry := range records {
		if entry.RevokedAt == nil && (filter.Symbol == "" || entry.Symbol == filter.Symbol) &&
			strings.Contains(strings.ToLower(entry.Thesis), filter.Query) &&
			(filter.MarketPhase == "" || strings.EqualFold(entry.MarketPhase, filter.MarketPhase)) &&
			(filter.Category == "" || entry.Category == filter.Category) && (filter.Outcome == "" || entry.Outcome == filter.Outcome) {
			matches = append(matches, entry)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].CreatedAt.After(matches[j].CreatedAt) })
	start := min(len(matches), filter.Offset)
	end := min(len(matches), start+50)
	return MemoryResults{Memories: matches[start:end], Total: len(matches), HasMore: end < len(matches)}, nil
}

func (s *FileHypothesisStore) memoryPath() string {
	return filepath.Join(s.dir, "research-memory.json")
}

func (s *FileHypothesisStore) readMemory() ([]ResearchMemory, error) {
	raw, err := os.ReadFile(s.memoryPath())
	if errors.Is(err, os.ErrNotExist) {
		return []ResearchMemory{}, nil
	}
	if err != nil {
		return nil, err
	}
	var records []ResearchMemory
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	if records == nil || len(records) > maxResearchMemory {
		return nil, errors.New("invalid persisted research memory")
	}
	seen := make(map[string]bool, len(records))
	for _, entry := range records {
		id, err := memoryID(entry.MemorySource)
		if err != nil || id != entry.ID || seen[id] || entry.CreatedAt.IsZero() || len(entry.Feedback) > maxMemoryFeedback {
			return nil, errors.New("invalid persisted memory entry")
		}
		seen[id] = true
	}
	return records, nil
}
