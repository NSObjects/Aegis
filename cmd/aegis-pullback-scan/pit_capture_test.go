package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

type fixtureCaptureProvider struct{}

func (fixtureCaptureProvider) RecentCompletedTradingSessions(context.Context, string) ([]string, error) {
	return []string{"20261119", "20261120"}, nil
}

func (fixtureCaptureProvider) ListStockUniverse(_ context.Context, date string) (technical.StockUniverse, error) {
	return technical.StockUniverse{AsOf: date, Source: "FIXTURE", Symbols: []string{"600519.SH", "000001.SZ"}}, nil
}

func TestCapturePITRecordsActualAvailabilityAndNeverOverwrites(t *testing.T) {
	current := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	start := current
	now := func() time.Time {
		at := current
		current = current.Add(time.Minute)
		return at
	}
	observe := func(_ context.Context, symbol, date string) (technical.HistoricalObservation, bool, error) {
		if symbol == "000001.SZ" {
			return technical.HistoricalObservation{}, false, nil
		}
		return technical.HistoricalObservation{Symbol: symbol, RequestedAsOf: date, EffectiveAsOf: "20261119", PriceBasis: "RAW",
			Bars:       []technical.HistoricalBar{{TradeDate: "20261119", Open: 100, High: 100, Low: 100, Close: 100, Volume: 100}},
			PITFactors: []technical.PITAdjustmentFactor{{TradeDate: "20261119", Factor: 1}},
		}, true, nil
	}
	capture, err := capturePIT(context.Background(), []string{"600519.SH", "000001.SZ"}, fixtureCaptureProvider{}, observe, now)
	if err != nil {
		t.Fatal(err)
	}
	row := capture.Members[1].Rows[0]
	if !capture.CalendarKnownAt.Equal(start.Add(time.Minute)) || !capture.UniverseKnownAt.Equal(start.Add(2*time.Minute)) ||
		!row.KnownAt.Equal(start.Add(4*time.Minute)) || !row.FactorKnownAt.Equal(row.KnownAt) ||
		!capture.Cutoff.Equal(start.Add(5*time.Minute)) || len(capture.Members[0].Rows) != 0 {
		t.Fatalf("capture did not preserve data availability or missingness: %+v", capture)
	}
	path := filepath.Join(t.TempDir(), "capture.json")
	if err := writeNewCapture(path, capture); err != nil {
		t.Fatal(err)
	}
	if err := writeNewCapture(path, capture); !errors.Is(err, os.ErrExist) {
		t.Fatalf("capture accepted an existing destination: %v", err)
	}
	var read pullback.PITSnapshot
	if err := decodeArchive(path, &read); err != nil {
		t.Fatal(err)
	}
	replay, err := pullback.ReplayPIT(read)
	if err != nil || len(replay.Results) != 2 || replay.Results[0].Verdict != pullback.InsufficientData {
		t.Fatalf("saved capture replay failed: %+v, %v", replay, err)
	}
}

func TestCapturePITRejectsIntraday(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 11, 20, 6, 59, 0, 0, time.UTC) }
	if _, err := capturePIT(context.Background(), nil, nil, nil, now); err == nil {
		t.Fatal("intraday capture succeeded before market close")
	}
}
