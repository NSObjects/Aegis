// Deterministic same-as-of comparison metrics and candidate ranking for the
// scan stage. Nothing here reads model confidence: every quantity derives
// from persisted bars and PIT factors under one strategy definition.
package pullback

import (
	"errors"
	"math"
	"sort"
	"time"

	"Aegis/internal/capability/technical"
)

// ComparisonVersion identifies the cross-symbol comparison contract; ranking
// inputs and tie-breaks are fixed so results are reproducible offline.
const ComparisonVersion = "candidate-comparison-v1"

// RankingRule: selected candidates are ordered by pullback proximity first
// (CloseOverMA20 ascending — deeper pullback toward MA20 ranks earlier), then
// trend margin (MA20OverMA60 descending), then symbol ascending.
const RankingRule = "PULLBACK_PROXIMITY_THEN_TREND_MARGIN"

// Metrics are same-as-of deterministic indicators for one symbol, computed
// with the exact moving-average formula the selection rule used. They are
// strategy quantities only, never a research verdict or trade signal.
type Metrics struct {
	Close         float64 `json:"close"`
	MA20          float64 `json:"ma20"`
	MA60          float64 `json:"ma60"`
	MA20OverMA60  float64 `json:"ma20_over_ma60"`
	CloseOverMA20 float64 `json:"close_over_ma20"`
	MA60Slope5    float64 `json:"ma60_slope_5d"`
}

// ErrMetricsInsufficientBars marks observations too short for the 60-session
// window; such symbols stay listed as incomparable instead of being scored.
var ErrMetricsInsufficientBars = errors.New("metrics require at least 65 aligned bars ending at as-of")

// ComputeMetrics derives the comparison indicators from validated bars and
// aligned PIT factors. Sharing adjustedSMA with Evaluate guarantees the
// published numbers can never drift from the rule that selected candidates.
func ComputeMetrics(asOf string, bars []technical.HistoricalBar, factors []AdjustmentFactor) (Metrics, error) {
	if _, err := time.Parse("20060102", asOf); err != nil || len(asOf) != 8 {
		return Metrics{}, errors.New("invalid as-of date")
	}
	if len(bars) < 65 || bars[len(bars)-1].TradeDate != asOf {
		return Metrics{}, ErrMetricsInsufficientBars
	}
	if len(factors) != len(bars) {
		return Metrics{}, errors.New("factors must align with bars")
	}
	anchor := factors[len(factors)-1].Factor
	n := len(bars)
	ma20 := adjustedSMA(bars, factors, anchor, n, 20)
	ma60 := adjustedSMA(bars, factors, anchor, n, 60)
	old60 := adjustedSMA(bars, factors, anchor, n-5, 60)
	m := Metrics{Close: bars[n-1].Close, MA20: ma20, MA60: ma60, MA20OverMA60: ma20 / ma60, CloseOverMA20: bars[n-1].Close / ma20, MA60Slope5: ma60/old60 - 1}
	for _, v := range []float64{m.Close, m.MA20, m.MA60, m.MA20OverMA60, m.CloseOverMA20, m.MA60Slope5} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return Metrics{}, errors.New("non-finite metric")
		}
	}
	return m, nil
}

// Candidate pairs one symbol with its same-as-of metrics for ranking.
type Candidate struct {
	Symbol  string
	Metrics Metrics
}

// RankCandidates applies RankingRule deterministically: pullback proximity
// ascending, trend margin descending, then symbol ascending as the final
// stable tie-break. The input slice is not mutated.
func RankCandidates(candidates []Candidate) []Candidate {
	out := append([]Candidate(nil), candidates...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Metrics.CloseOverMA20 != b.Metrics.CloseOverMA20 {
			return a.Metrics.CloseOverMA20 < b.Metrics.CloseOverMA20
		}
		if a.Metrics.MA20OverMA60 != b.Metrics.MA20OverMA60 {
			return a.Metrics.MA20OverMA60 > b.Metrics.MA20OverMA60
		}
		return a.Symbol < b.Symbol
	})
	return out
}
