package application

import (
	"errors"
	"sort"

	"Aegis/internal/strategy/pullback"
)

// ScanComparison is the deterministic cross-symbol view over one completed
// scan. Every ranked row cites the digest of the exact persisted observation
// its metrics came from, so any ranking can be re-verified offline through the
// existing snapshot endpoint. Model confidence never participates.
type ScanComparison struct {
	ScanID            string                       `json:"scan_id"`
	AsOf              string                       `json:"as_of"`
	StrategyVersion   string                       `json:"strategy_version"`
	ComparisonVersion string                       `json:"comparison_version"`
	RankingRule       string                       `json:"ranking_rule"`
	Ranked            []ScanComparisonRanked       `json:"ranked"`
	Incomparable      []ScanComparisonIncomparable `json:"incomparable"`
}

// ScanComparisonSource anchors one row to its persisted input observation.
type ScanComparisonSource struct {
	SHA256        string `json:"sha256"`
	BarsCount     int    `json:"bars_count"`
	EffectiveAsOf string `json:"effective_as_of"`
}

type ScanComparisonRanked struct {
	Rank    int                  `json:"rank"`
	Symbol  string               `json:"symbol"`
	Metrics pullback.Metrics     `json:"metrics"`
	Source  ScanComparisonSource `json:"source"`
}

// ScanComparisonIncomparable lists, with an explicit reason, every universe
// member that cannot join the same-as-of ranking. Absence is stated, never
// silently dropped.
type ScanComparisonIncomparable struct {
	Symbol  string                `json:"symbol"`
	Verdict pullback.Verdict      `json:"verdict"`
	Reason  string                `json:"reason"`
	Note    string                `json:"note"`
	Source  *ScanComparisonSource `json:"source,omitempty"`
}

func comparisonIncomparableNote(result pullback.Result) string {
	if result.Verdict != pullback.InsufficientData {
		return "NOT_SELECTED"
	}
	switch result.Reason {
	case "MISSING_BARS":
		return "METRICS_INSUFFICIENT_BARS"
	case "STALE_OR_MISSING_CLOSE":
		return "STALE_OR_MISSING_CLOSE"
	case "UNSUPPORTED_PRICE_BASIS":
		return "UNSUPPORTED_PRICE_BASIS"
	case "MISSING_PIT_ADJUSTMENT":
		return "MISSING_PIT_ADJUSTMENT"
	case "INVALID_PIT_ADJUSTMENT":
		return "INVALID_PIT_ADJUSTMENT"
	case "HALTED_OR_NO_VOLUME":
		return "HALTED_OR_NO_VOLUME"
	default:
		return "INSUFFICIENT_DATA"
	}
}

// Comparison builds the ranked view for a completed scan. Unfinished scans,
// missing snapshots and digest failures fail closed; a digest mismatch means
// the persisted inputs no longer match the record and the whole comparison is
// refused rather than served on unverifiable data.
func (m *ScanManager) Comparison(scanID string) (ScanComparison, error) {
	if m == nil {
		return ScanComparison{}, errors.New("nil scan manager")
	}
	record, err := m.store.Get(scanID)
	if err != nil {
		return ScanComparison{}, err
	}
	if record.State != ScanCompleted {
		return ScanComparison{}, ErrScanNotCompleted
	}
	refs := make(map[string]ScanSnapshotRef, len(record.Snapshots))
	for _, ref := range record.Snapshots {
		refs[ref.Symbol] = ref
	}
	verdicts := make(map[string]pullback.Result, len(record.Results))
	for _, r := range record.Results {
		verdicts[r.Symbol] = r
	}
	candidates := make([]pullback.Candidate, 0, len(record.Symbols))
	rankedSources := make(map[string]ScanComparisonSource)
	incomparable := make([]ScanComparisonIncomparable, 0, len(record.Symbols))
	for _, symbol := range record.Symbols {
		result := verdicts[symbol]
		ref, hasSnapshot := refs[symbol]
		if !hasSnapshot {
			incomparable = append(incomparable, ScanComparisonIncomparable{Symbol: symbol, Verdict: result.Verdict, Reason: result.Reason, Note: "NO_COMMITTED_SNAPSHOT"})
			continue
		}
		obs, err := m.store.GetSnapshot(scanID, symbol, ref.SHA256)
		if err != nil {
			return ScanComparison{}, err
		}
		source := ScanComparisonSource{SHA256: ref.SHA256, BarsCount: ref.BarsCount, EffectiveAsOf: ref.EffectiveAsOf}

		// Non-selected observations do not need ranking metrics. Preserve their
		// actual exclusion reason instead of trying to compute ratios from
		// missing/misaligned PIT factors and failing the entire comparison.
		if result.Verdict != pullback.Selected {
			incomparable = append(incomparable, ScanComparisonIncomparable{
				Symbol: symbol, Verdict: result.Verdict, Reason: result.Reason,
				Note: comparisonIncomparableNote(result), Source: &source,
			})
			continue
		}

		// A selected verdict implies validated RAW bars, PIT factors, and an
		// exact as-of close. If those invariants no longer hold, fail closed.
		factors := make([]pullback.AdjustmentFactor, len(obs.PITFactors))
		for i, f := range obs.PITFactors {
			factors[i] = pullback.AdjustmentFactor{TradeDate: f.TradeDate, Factor: f.Factor}
		}
		metrics, mErr := pullback.ComputeMetrics(record.AsOf, obs.Bars, factors)
		if mErr != nil {
			return ScanComparison{}, mErr
		}
		candidates = append(candidates, pullback.Candidate{Symbol: symbol, Metrics: metrics})
		rankedSources[symbol] = source
	}
	ordered := pullback.RankCandidates(candidates)
	ranked := make([]ScanComparisonRanked, 0, len(ordered))
	for i, c := range ordered {
		ranked = append(ranked, ScanComparisonRanked{Rank: i + 1, Symbol: c.Symbol, Metrics: c.Metrics, Source: rankedSources[c.Symbol]})
	}
	sort.Slice(incomparable, func(i, j int) bool { return incomparable[i].Symbol < incomparable[j].Symbol })
	return ScanComparison{
		ScanID: record.ID, AsOf: record.AsOf, StrategyVersion: record.StrategyVersion,
		ComparisonVersion: pullback.ComparisonVersion, RankingRule: pullback.RankingRule,
		Ranked: ranked, Incomparable: incomparable,
	}, nil
}
