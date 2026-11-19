package application

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type HypothesisCloseRule struct {
	TradeDate string  `json:"trade_date"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
}

type HypothesisDocumentRule struct {
	DocumentID string `json:"document_id"`
	OnMatch    string `json:"on_match"`
}

type HypothesisObservation struct {
	RunID          string    `json:"run_id"`
	AsOf           time.Time `json:"as_of"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	Status         string    `json:"status"`
	Reason         string    `json:"reason"`
	TradeDate      string    `json:"trade_date,omitempty"`
	Close          *float64  `json:"close,omitempty"`
	DocumentID     string    `json:"document_id,omitempty"`
}

var (
	ErrHypothesisNotFound = errors.New("research hypothesis not found")
	ErrObservationSource  = errors.New("observation requires a later completed report for the same symbol")
)

func hypothesisMarketClose(date string) (time.Time, error) {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	day, err := time.ParseInLocation("20060102", date, zone)
	return day.Add(15 * time.Hour), err
}

func validHypothesisPrice(price float64) bool {
	return price > 0 && !math.IsInf(price, 0)
}

func evaluateHypothesis(item ResearchHypothesis, run RunRecord) HypothesisObservation {
	result := HypothesisObservation{
		RunID: run.ID, AsOf: run.RequestedAsOf.UTC(), ArtifactSHA256: artifactDigest(run.Artifact),
		Status: "INCONCLUSIVE", Reason: "NO_STRUCTURED_RULE",
	}
	if result.AsOf.After(item.ExpiresAt) {
		result.Status = "EXPIRED"
	}
	if rule := item.DocumentRule; rule != nil {
		return evaluateHypothesisDocument(result, item, run, *rule)
	}
	rule := item.CloseRule
	if rule == nil {
		return result
	}
	result.TradeDate = rule.TradeDate
	result.Reason = "MARKET_CLOSE_NOT_REACHED"
	closeAt, err := hypothesisMarketClose(rule.TradeDate)
	if err != nil || result.AsOf.Before(closeAt) {
		return result
	}

	result.Reason = "CLOSE_UNAVAILABLE"
	series := run.HistoricalOHLCV
	if series == nil || series.Symbol != item.Symbol || series.PriceBasis != "RAW" || series.EffectiveAsOf < rule.TradeDate {
		return result
	}
	for _, bar := range series.Bars {
		if bar.TradeDate != rule.TradeDate {
			continue
		}
		if result.Close != nil || !validHypothesisPrice(bar.Close) {
			result.Close = nil
			return result
		}
		price := bar.Close
		result.Close = &price
	}
	if result.Close == nil {
		return result
	}
	result.Status, result.Reason = "REFUTED", "PRICE_CRITERION"
	if (rule.Operator == "GTE" && *result.Close >= rule.Threshold) ||
		(rule.Operator == "LTE" && *result.Close <= rule.Threshold) {
		result.Status = "SUPPORTED"
	}
	return result
}

func evaluateHypothesisDocument(result HypothesisObservation, item ResearchHypothesis, run RunRecord, rule HypothesisDocumentRule) HypothesisObservation {
	result.Reason = "DOCUMENT_UNAVAILABLE"
	index := run.EvidenceIndex
	if index == nil || !slices.Contains(index.DocumentIDs, rule.DocumentID) {
		return result
	}
	for _, doc := range index.Documents {
		if doc.DocumentID != rule.DocumentID || !doc.IsPrimary || doc.PublishedAt == nil || doc.AvailableAt == nil {
			continue
		}
		if doc.PublishedAt.After(item.AsOf) && !doc.AvailableAt.Before(*doc.PublishedAt) &&
			!doc.AvailableAt.After(item.ExpiresAt) && !doc.AvailableAt.After(result.AsOf) {
			result.Status = rule.OnMatch
			result.DocumentID = doc.DocumentID
			result.Reason = "EVENT_DOCUMENT"
			return result
		}
	}
	return result
}

func (s *FileHypothesisStore) Observations(sourceRunID, hypothesisID string) ([]HypothesisObservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.find(sourceRunID, hypothesisID)
	if err != nil {
		return nil, err
	}
	return s.readObservations(item.ID)
}

func (s *FileHypothesisStore) Observe(sourceRunID, hypothesisID string, run RunRecord) (HypothesisObservation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.find(sourceRunID, hypothesisID)
	if err != nil {
		return HypothesisObservation{}, false, err
	}
	if run.State != RunCompleted || !validRunID(run.ID) || run.ID == sourceRunID || run.Symbol != item.Symbol ||
		!run.RequestedAsOf.After(item.AsOf) || len(run.Artifact) == 0 {
		return HypothesisObservation{}, false, ErrObservationSource
	}
	records, err := s.readObservations(item.ID)
	if err != nil {
		return HypothesisObservation{}, false, err
	}
	for _, record := range records {
		if record.RunID == run.ID {
			return record, false, nil
		}
	}
	if len(records) >= maxRunHypotheses {
		return HypothesisObservation{}, false, ErrHypothesisCapacity
	}
	record := evaluateHypothesis(item, run)
	if record.ArtifactSHA256 == "" || (run.ArtifactSHA256 != "" && record.ArtifactSHA256 != run.ArtifactSHA256) {
		return HypothesisObservation{}, false, ErrObservationSource
	}
	if err := writeJSONFile(s.observationPath(item.ID), append(records, record)); err != nil {
		return HypothesisObservation{}, false, err
	}
	return record, true, nil
}

func (s *FileHypothesisStore) find(runID, hypothesisID string) (ResearchHypothesis, error) {
	if !validRunID(runID) || !validRunID(hypothesisID) {
		return ResearchHypothesis{}, ErrHypothesisNotFound
	}
	items, err := s.read(runID)
	if err != nil {
		return ResearchHypothesis{}, err
	}
	for _, item := range items {
		if item.ID == hypothesisID {
			return item, nil
		}
	}
	return ResearchHypothesis{}, ErrHypothesisNotFound
}

func (s *FileHypothesisStore) observationPath(id string) string {
	return filepath.Join(s.dir, "observations-"+id+".json")
}

func (s *FileHypothesisStore) readObservations(id string) ([]HypothesisObservation, error) {
	records := []HypothesisObservation{}
	raw, err := os.ReadFile(s.observationPath(id))
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
		return nil, errors.New("invalid persisted observations")
	}
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		if !validRunID(record.RunID) || record.AsOf.IsZero() || len(record.ArtifactSHA256) != 64 || seen[record.RunID] {
			return nil, errors.New("invalid persisted observation")
		}
		switch record.Status {
		case "SUPPORTED", "REFUTED":
			if record.Close == nil && record.DocumentID == "" {
				return nil, errors.New("persisted verdict lacks an observation")
			}
		case "INCONCLUSIVE", "EXPIRED":
		default:
			return nil, errors.New("invalid persisted observation status")
		}
		seen[record.RunID] = true
	}
	return records, nil
}
