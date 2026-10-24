package application

import (
	"errors"

	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

// ScanReplay is the machine-checkable replay verdict for one completed scan:
// re-running the same strategy version over the exact persisted observations
// must reproduce the persisted results symbol for symbol. Mismatches are
// reported, never silently repaired.
type ScanReplay struct {
	ScanID          string                   `json:"scan_id"`
	AsOf            string                   `json:"as_of"`
	StrategyVersion string                   `json:"strategy_version"`
	Replayable      bool                     `json:"replayable"`
	Matches         bool                     `json:"matches"`
	Rows            []ScanReplayRow          `json:"rows"`
	Unreplayable    []ScanReplayUnreplayable `json:"unreplayable"`
}

type ScanReplayRow struct {
	Symbol    string               `json:"symbol"`
	Persisted pullback.Result      `json:"persisted"`
	Replayed  pullback.Result      `json:"replayed"`
	Match     bool                 `json:"match"`
	Source    ScanComparisonSource `json:"source"`
}

type ScanReplayUnreplayable struct {
	Symbol string `json:"symbol"`
	Reason string `json:"reason"`
}

// ReplayScan re-derives every result from the persisted input snapshots under
// the record's own PIT cutoff and compares with what was persisted. A missing
// snapshot or a digest mismatch fails closed: replaying unverifiable inputs
// would fabricate reproducibility. Unreplayable symbols (no committed
// snapshot, e.g. NO_VALID_HISTORICAL_OBSERVATION) are listed explicitly.
func (m *ScanManager) ReplayScan(scanID string) (ScanReplay, error) {
	if m == nil {
		return ScanReplay{}, errors.New("nil scan manager")
	}
	record, err := m.store.Get(scanID)
	if err != nil {
		return ScanReplay{}, err
	}
	if record.State != ScanCompleted {
		return ScanReplay{}, ErrScanNotCompleted
	}
	verdicts := make(map[string]pullback.Result, len(record.Results))
	for _, r := range record.Results {
		verdicts[r.Symbol] = r
	}
	refs := make(map[string]ScanSnapshotRef, len(record.Snapshots))
	for _, ref := range record.Snapshots {
		refs[ref.Symbol] = ref
	}
	replay := ScanReplay{ScanID: record.ID, AsOf: record.AsOf, StrategyVersion: record.StrategyVersion}
	observations := make([]technical.HistoricalObservation, 0, len(record.Symbols))
	for _, symbol := range record.Symbols {
		ref, ok := refs[symbol]
		if !ok {
			replay.Unreplayable = append(replay.Unreplayable, ScanReplayUnreplayable{Symbol: symbol, Reason: "NO_COMMITTED_SNAPSHOT"})
			continue
		}
		obs, err := m.store.GetSnapshot(scanID, symbol, ref.SHA256)
		if err != nil {
			return ScanReplay{}, err
		}
		if obs.RequestedAsOf != record.AsOf || obs.EffectiveAsOf != record.AsOf {
			return ScanReplay{}, errors.New("snapshot as-of does not match scan cutoff")
		}
		replay.Rows = append(replay.Rows, ScanReplayRow{Symbol: symbol, Persisted: verdicts[symbol], Source: ScanComparisonSource{SHA256: ref.SHA256, BarsCount: ref.BarsCount, EffectiveAsOf: ref.EffectiveAsOf}})
		observations = append(observations, obs)
	}
	if len(observations) > 0 {
		replayed, err := pullback.Scan(record.AsOf, observations)
		if err != nil {
			return ScanReplay{}, err
		}
		bySymbol := make(map[string]pullback.Result, len(replayed))
		for _, r := range replayed {
			bySymbol[r.Symbol] = r
		}
		for i := range replay.Rows {
			row := &replay.Rows[i]
			row.Replayed = bySymbol[row.Symbol]
			row.Match = row.Replayed == row.Persisted
		}
	}
	replay.Replayable = true
	replay.Matches = true
	for _, row := range replay.Rows {
		if !row.Match {
			replay.Matches = false
		}
	}
	return replay, nil
}

// Replay is the ScanController-facing alias for ReplayScan.
func (m *ScanManager) Replay(scanID string) (ScanReplay, error) { return m.ReplayScan(scanID) }
