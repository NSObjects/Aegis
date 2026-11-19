package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"Aegis/internal/strategy/pullback"
)

const maxMemoryFeedback = 8

var ErrFeedbackUnavailable = errors.New("feedback requires a matching out-of-sample candidate within the source and observation cutoffs")

// ResearchFeedback records one independently labeled or replayed sample.
// The archive is retained by its owner and rechecked by digest for replay.
type ResearchFeedback struct {
	ArchiveSHA256 string                     `json:"archive_sha256"`
	RecordedAt    time.Time                  `json:"recorded_at"`
	Strategy      string                     `json:"strategy_version"`
	DefinitionSHA string                     `json:"strategy_definition_sha256"`
	SnapshotSHA   string                     `json:"snapshot_sha256"`
	SignalCutoff  time.Time                  `json:"signal_cutoff"`
	Through       string                     `json:"test_through"`
	Direction     string                     `json:"direction"`
	Selection     string                     `json:"selection"`
	DataQuality   string                     `json:"data_quality"`
	Timing        string                     `json:"timing"`
	Signal        pullback.Result            `json:"signal"`
	Outcome       *pullback.CandidateOutcome `json:"candidate_outcome,omitempty"`
	TradeStatus   string                     `json:"trade_status,omitempty"`
}

var feedbackZone = time.FixedZone("Asia/Shanghai", 8*60*60)

func (s *FileHypothesisStore) AddResearchFeedback(memory string, input pullback.ValidationInput, now time.Time) (ResearchFeedback, bool, error) {
	if !validRunID(memory) || now.IsZero() {
		return ResearchFeedback{}, false, ErrInvalidMemory
	}
	item, observation, err := s.feedbackSource(memory)
	if err != nil {
		return ResearchFeedback{}, false, err
	}
	feedback, err := evaluateResearchFeedback(item, observation, input, now.UTC())
	if err != nil {
		return ResearchFeedback{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.readMemory()
	if err != nil {
		return ResearchFeedback{}, false, err
	}
	for i := range records {
		record := &records[i]
		if record.ID != memory {
			continue
		}
		if record.RevokedAt != nil {
			return ResearchFeedback{}, false, ErrMemoryRevoked
		}
		for _, previous := range record.Feedback {
			if previous.ArchiveSHA256 == feedback.ArchiveSHA256 {
				return previous, false, nil
			}
		}
		if len(record.Feedback) >= maxMemoryFeedback {
			return ResearchFeedback{}, false, ErrMemoryCapacity
		}
		record.Feedback = append(record.Feedback, feedback)
		if err := writeJSONFile(s.memoryPath(), records); err != nil {
			return ResearchFeedback{}, false, err
		}
		return feedback, true, nil
	}
	return ResearchFeedback{}, false, ErrMemoryNotFound
}

func (s *FileHypothesisStore) feedbackSource(id string) (ResearchHypothesis, HypothesisObservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.readMemory()
	if err != nil {
		return ResearchHypothesis{}, HypothesisObservation{}, err
	}
	for _, record := range records {
		if record.ID != id {
			continue
		}
		if record.RevokedAt != nil {
			return ResearchHypothesis{}, HypothesisObservation{}, ErrMemoryRevoked
		}
		item, err := s.find(record.SourceRunID, record.HypothesisID)
		if err != nil {
			return ResearchHypothesis{}, HypothesisObservation{}, err
		}
		observations, err := s.readObservations(item.ID)
		if err != nil {
			return ResearchHypothesis{}, HypothesisObservation{}, err
		}
		for _, observation := range observations {
			if observation.RunID == record.ObservationRunID {
				return item, observation, nil
			}
		}
		return ResearchHypothesis{}, HypothesisObservation{}, ErrMemorySource
	}
	return ResearchHypothesis{}, HypothesisObservation{}, ErrMemoryNotFound
}

func evaluateResearchFeedback(item ResearchHypothesis, obs HypothesisObservation, input pullback.ValidationInput, now time.Time) (ResearchFeedback, error) {
	date := item.AsOf.In(feedbackZone).Format("20060102")
	for _, snapshot := range input.Backtest.Signals {
		if snapshot.Cutoff.In(feedbackZone).Format("20060102") != date {
			continue
		}
		// A snapshot after the original research cutoff cannot evaluate that
		// original decision. Likewise, feedback cannot use future observations.
		if snapshot.Cutoff.After(item.AsOf) || obs.AsOf.After(now) {
			return ResearchFeedback{}, ErrFeedbackUnavailable
		}
		report, err := pullback.ValidateBacktest(input)
		if err != nil {
			return ResearchFeedback{}, ErrInvalidMemory
		}
		for _, fold := range report.Folds {
			if date < fold.Range.TestFrom || date > fold.Range.TestThrough {
				continue
			}
			day, err := time.ParseInLocation("20060102", fold.Range.TestThrough, feedbackZone)
			if err != nil || day.AddDate(0, 0, 1).After(obs.AsOf) {
				return ResearchFeedback{}, ErrFeedbackUnavailable
			}
			feedback, err := sampleFeedback(fold.OutOfSample, date, item.Symbol)
			if err != nil {
				return ResearchFeedback{}, err
			}
			feedback.Direction = "UNASSESSED"
			if item.CloseRule != nil && obs.Reason == "PRICE_CRITERION" && obs.Close != nil {
				feedback.Direction = obs.Status
			}
			for _, outcome := range input.Outcomes {
				if outcome.Date == date && outcome.Symbol == item.Symbol &&
					!outcome.KnownAt.Before(day.AddDate(0, 0, 1)) {
					feedback.Timing = "LABEL_AFTER_TEST_WINDOW"
				}
			}
			raw, err := json.Marshal(input)
			if err != nil {
				return ResearchFeedback{}, ErrInvalidMemory
			}
			digest := sha256.Sum256(raw)
			feedback.ArchiveSHA256 = hex.EncodeToString(digest[:])
			feedback.RecordedAt, feedback.SignalCutoff, feedback.Through = now, snapshot.Cutoff.UTC(), fold.Range.TestThrough
			return feedback, nil
		}
	}
	return ResearchFeedback{}, ErrFeedbackUnavailable
}

func sampleFeedback(window pullback.WindowReport, date, symbol string) (ResearchFeedback, error) {
	for _, replay := range window.Replay.Signals {
		if replay.AsOf != date {
			continue
		}
		for _, candidate := range replay.Results {
			if candidate.Symbol != symbol {
				continue
			}
			entry := ResearchFeedback{
				Strategy: candidate.Version, DefinitionSHA: replay.DefinitionSHA256, SnapshotSHA: replay.SnapshotSHA256,
				Signal: candidate, Selection: "UNASSESSED", Timing: "ARCHIVE_CUTOFF_CHECKED", DataQuality: "EVALUABLE",
			}
			if candidate.Verdict == pullback.InsufficientData {
				entry.DataQuality = candidate.Reason
			}
			for _, label := range window.CandidateOutcomes {
				if label.Date != date || label.Symbol != symbol {
					continue
				}
				entry.Outcome = &label
				switch {
				case candidate.Verdict == pullback.Selected && *label.Positive:
					entry.Selection = "TRUE_POSITIVE"
				case candidate.Verdict == pullback.Selected:
					entry.Selection = "FALSE_POSITIVE"
				case *label.Positive:
					entry.Selection = "FALSE_NEGATIVE"
				default:
					entry.Selection = "TRUE_NEGATIVE"
				}
			}
			for _, trade := range window.Replay.Trades {
				if trade.SignalDate == date && trade.Symbol == symbol {
					entry.TradeStatus = trade.Status
					break
				}
			}
			return entry, nil
		}
	}
	return ResearchFeedback{}, ErrFeedbackUnavailable
}
