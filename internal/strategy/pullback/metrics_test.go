package pullback

import (
	"math"
	"testing"

	"Aegis/internal/capability/technical"
)

func TestComputeMetricsMatchesSelectionRuleOnSelectedCandidate(t *testing.T) {
	bars := risingBars()
	n := len(bars) - 1
	// Same shaping as the scan test: pull the last close back near MA20.
	bars[n].Open = 129
	bars[n].High = 130
	bars[n].Low = 128
	bars[n].Close = 129
	asOf := bars[n].TradeDate
	verdict, err := Evaluate("600519.SH", asOf, bars, sameFactors(bars))
	if err != nil || verdict.Verdict != Selected {
		t.Fatalf("fixture must select: %+v %v", verdict, err)
	}
	m, err := ComputeMetrics(asOf, bars, sameFactors(bars))
	if err != nil {
		t.Fatal(err)
	}
	if m.Close != 129 {
		t.Fatalf("close metric %v", m.Close)
	}
	// The published ratios must reproduce the rule's own bounds, proving the
	// shared formula keeps comparison and selection on one definition.
	if m.MA20OverMA60 <= 1 || m.CloseOverMA20 < 1 || m.CloseOverMA20 > 1.03 {
		t.Fatalf("metrics disagree with SELECTED bounds: %+v", m)
	}
	if m.MA60Slope5 <= 0 {
		t.Fatalf("rising fixture must yield positive MA60 slope: %+v", m)
	}
	expected := m.Close / m.MA20
	if math.Abs(expected-m.CloseOverMA20) > 1e-12 {
		t.Fatalf("ratio inconsistent: %v vs %v", expected, m.CloseOverMA20)
	}
}

func TestComputeMetricsRejectsInsufficientOrMisalignedInputs(t *testing.T) {
	bars := risingBars()
	asOf := bars[len(bars)-1].TradeDate
	short := bars[:64]
	if _, err := ComputeMetrics(asOf, short, sameFactors(short)); err != ErrMetricsInsufficientBars {
		t.Fatalf("short bars err=%v", err)
	}
	stale := append([]technical.HistoricalBar(nil), bars...)
	stale[len(stale)-1].TradeDate = "20260101"
	if _, err := ComputeMetrics(asOf, stale, sameFactors(stale)); err != ErrMetricsInsufficientBars {
		t.Fatalf("stale last bar err=%v", err)
	}
	if _, err := ComputeMetrics(asOf, bars, sameFactors(bars)[:10]); err == nil {
		t.Fatal("misaligned factors accepted")
	}
	if _, err := ComputeMetrics("2026-1-1", bars, sameFactors(bars)); err == nil {
		t.Fatal("invalid as-of accepted")
	}
}

func TestRankCandidatesIsDeterministicPullbackFirst(t *testing.T) {
	deep := Candidate{Symbol: "600519.SH", Metrics: Metrics{CloseOverMA20: 1.001, MA20OverMA60: 1.05}}
	shallow := Candidate{Symbol: "000001.SZ", Metrics: Metrics{CloseOverMA20: 1.028, MA20OverMA60: 1.30}}
	ranked := RankCandidates([]Candidate{shallow, deep})
	if ranked[0].Symbol != "600519.SH" || ranked[1].Symbol != "000001.SZ" {
		t.Fatalf("pullback proximity must lead: %+v", ranked)
	}
	// Equal proximity falls back to stronger trend margin, then symbol order.
	a := Candidate{Symbol: "300750.SZ", Metrics: Metrics{CloseOverMA20: 1.01, MA20OverMA60: 1.10}}
	b := Candidate{Symbol: "000858.SZ", Metrics: Metrics{CloseOverMA20: 1.01, MA20OverMA60: 1.20}}
	c := Candidate{Symbol: "600036.SH", Metrics: Metrics{CloseOverMA20: 1.01, MA20OverMA60: 1.20}}
	ranked = RankCandidates([]Candidate{a, b, c})
	if ranked[0].Symbol != "000858.SZ" || ranked[1].Symbol != "600036.SH" || ranked[2].Symbol != "300750.SZ" {
		t.Fatalf("tie-break order wrong: %+v", ranked)
	}
	// Repeated runs and shuffled inputs give the same order; input untouched.
	first := RankCandidates([]Candidate{c, a, b})
	second := RankCandidates([]Candidate{b, c, a})
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("ranking not deterministic")
		}
	}
	again := []Candidate{c, a, b}
	_ = RankCandidates(again)
	if again[0].Symbol != "600036.SH" {
		t.Fatal("input slice mutated")
	}
}
