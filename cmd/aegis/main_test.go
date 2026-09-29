package main

import (
	eventcap "Aegis/internal/capability/event"
	"testing"
	"time"
)

func TestNewLiveResearchInputPreservesPITContract(
	t *testing.T,
) {
	location := time.FixedZone(
		"Asia/Shanghai",
		8*60*60,
	)
	cutoff := time.Date(
		2026, 9, 29, 21, 0, 0, 0,
		location,
	)
	lookback := 7 * 24 * time.Hour

	input, err := newLiveResearchInput(
		" 600519.sh ",
		"  custom market query ",
		cutoff,
		lookback,
	)
	if err != nil {
		t.Fatalf(
			"newLiveResearchInput() error = %v",
			err,
		)
	}

	if input.Symbol != "600519.SH" {
		t.Fatalf(
			"symbol = %q",
			input.Symbol,
		)
	}
	if input.MarketQuery !=
		"custom market query" {
		t.Fatalf(
			"market query = %q",
			input.MarketQuery,
		)
	}

	task := input.EventInput.Task
	if task.Symbol != input.Symbol {
		t.Fatalf(
			"event task symbol = %q, want %q",
			task.Symbol,
			input.Symbol,
		)
	}
	if task.InformationNeed !=
		eventcap.NeedEventEvidence {
		t.Fatalf(
			"information need = %q",
			task.InformationNeed,
		)
	}
	if !task.AsOf.Equal(cutoff) {
		t.Fatalf(
			"as_of = %s, want %s",
			task.AsOf,
			cutoff,
		)
	}
	if !task.TimeRange.End.Equal(cutoff) ||
		!task.TimeRange.Start.Equal(
			cutoff.Add(-lookback),
		) {
		t.Fatalf(
			"time range = %+v",
			task.TimeRange,
		)
	}
	if !task.Requirements.PITSafe {
		t.Fatal(
			"live event research task must be PIT-safe",
		)
	}
	if task.Requirements.MinDocuments != 1 ||
		task.Requirements.MinIndependentSources != 1 {
		t.Fatalf(
			"requirements = %+v",
			task.Requirements,
		)
	}
	if task.Requirements.RequirePrimarySource {
		t.Fatal(
			"live contract must not require a primary source",
		)
	}

	preferred := map[eventcap.SourceType]bool{}
	for _, sourceType :=
		range task.Requirements.PreferredSourceTypes {
		preferred[sourceType] = true
	}
	for _, want := range []eventcap.SourceType{
		eventcap.SourceOfficial,
		eventcap.SourceIndustryMedia,
		eventcap.SourceMainstreamMedia,
	} {
		if !preferred[want] {
			t.Fatalf(
				"preferred source types = %+v; missing %q",
				task.Requirements.PreferredSourceTypes,
				want,
			)
		}
	}
}

func TestNewLiveResearchInputRejectsInvalidBoundary(
	t *testing.T,
) {
	cutoff := time.Date(
		2026, 9, 29, 12, 0, 0, 0,
		time.UTC,
	)

	tests := []struct {
		name     string
		symbol   string
		cutoff   time.Time
		lookback time.Duration
	}{
		{
			name:     "empty symbol",
			cutoff:   cutoff,
			lookback: 24 * time.Hour,
		},
		{
			name:     "zero cutoff",
			symbol:   "600519.SH",
			lookback: 24 * time.Hour,
		},
		{
			name:     "non-positive lookback",
			symbol:   "600519.SH",
			cutoff:   cutoff,
			lookback: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err :=
				newLiveResearchInput(
					test.symbol,
					"",
					test.cutoff,
					test.lookback,
				); err == nil {
				t.Fatal(
					"expected input validation error",
				)
			}
		})
	}
}
