package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

type pitCaptureProvider interface {
	RecentCompletedTradingSessions(context.Context, string) ([]string, error)
	ListStockUniverse(context.Context, string) (technical.StockUniverse, error)
}

type captureObservation func(context.Context, string, string) (technical.HistoricalObservation, bool, error)

func capturePITFile(ctx context.Context, path, requested string) error {
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return errors.New("capture destination must be a new file")
	}
	symbols, err := parseSymbols(requested)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(os.Getenv("TUSHARE_TOKEN"))
	if token == "" {
		return errors.New("TUSHARE_TOKEN required for live capture")
	}
	provider, err := technical.NewTushareClient(token)
	if err != nil {
		return err
	}
	researcher, err := technical.NewHTTPResearcher(token)
	if err != nil {
		return err
	}
	snapshot, err := capturePIT(ctx, symbols, provider, func(ctx context.Context, symbol, date string) (technical.HistoricalObservation, bool, error) {
		return technical.ObserveHistorical(ctx, researcher, technical.Request{Symbol: symbol, AsOf: date})
	}, time.Now)
	if err != nil {
		return err
	}
	return writeNewCapture(path, snapshot)
}

func capturePIT(ctx context.Context, symbols []string, provider pitCaptureProvider, observe captureObservation, now func() time.Time) (pullback.PITSnapshot, error) {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := now().In(zone)
	asOf := start.Format("20060102")
	if start.Hour() < 15 {
		return pullback.PITSnapshot{}, errors.New("same-day capture requires Shanghai market close")
	}
	sessions, err := provider.RecentCompletedTradingSessions(ctx, asOf)
	if err != nil {
		return pullback.PITSnapshot{}, errors.New("cannot capture completed trading calendar")
	}
	calendarKnown := now().UTC()
	universe, err := provider.ListStockUniverse(ctx, asOf)
	if err != nil || universe.AsOf != asOf {
		return pullback.PITSnapshot{}, errors.New("cannot capture the current eligible universe")
	}
	universeKnown := now().UTC()
	eligible := make(map[string]bool, len(universe.Symbols))
	for _, symbol := range universe.Symbols {
		eligible[symbol] = true
	}
	requested := append([]string(nil), symbols...)
	sort.Strings(requested)
	snapshot := pullback.PITSnapshot{
		StrategyVersion: pullback.StrategyVersion,
		CalendarSource:  "TUSHARE_SSE_TRADE_CAL", CalendarKnownAt: calendarKnown, Sessions: sessions,
		UniverseSource: universe.Source + ":EXPLICIT_SUBSET", UniverseKnownAt: universeKnown,
		Members: make([]pullback.PITUniverseEntry, 0, len(symbols)),
	}
	sessionSet := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		sessionSet[session] = true
	}
	for _, symbol := range requested {
		if !eligible[symbol] {
			return pullback.PITSnapshot{}, fmt.Errorf("requested symbol %s is outside the current eligible universe", symbol)
		}
		observation, ok, err := observe(ctx, symbol, asOf)
		if err != nil {
			return pullback.PITSnapshot{}, fmt.Errorf("cannot capture technical data for %s", symbol)
		}
		available := now().UTC()
		member := pullback.PITUniverseEntry{Symbol: symbol, Rows: []pullback.PITRow{}}
		if ok {
			if observation.Symbol != symbol || observation.RequestedAsOf != asOf || observation.PriceBasis != "RAW" {
				return pullback.PITSnapshot{}, fmt.Errorf("unexpected observation identity or price basis for %s", symbol)
			}
			factors := make(map[string]float64, len(observation.PITFactors))
			for _, factor := range observation.PITFactors {
				factors[factor.TradeDate] = factor.Factor
			}
			for _, bar := range observation.Bars {
				if !sessionSet[bar.TradeDate] || bar.Volume <= 0 {
					continue
				}
				row := pullback.PITRow{Session: bar.TradeDate, State: "TRADED", KnownAt: available, Bar: &bar}
				if factor, exists := factors[bar.TradeDate]; exists {
					row.Factor, row.FactorKnownAt = &factor, &available
				}
				member.Rows = append(member.Rows, row)
			}
		}
		snapshot.Members = append(snapshot.Members, member)
	}
	snapshot.Cutoff = now().UTC()
	if snapshot.Cutoff.In(zone).Format("20060102") != asOf {
		return pullback.PITSnapshot{}, errors.New("capture crossed the Shanghai date boundary")
	}
	if _, err := pullback.ReplayPIT(snapshot); err != nil {
		return pullback.PITSnapshot{}, fmt.Errorf("captured archive did not validate: %w", err)
	}
	return snapshot, nil
}

func writeNewCapture(path string, snapshot pullback.PITSnapshot) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".pit-capture-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = json.NewEncoder(file).Encode(snapshot); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	// A hard link publishes the completed file atomically and fails if the
	// path already exists, including when two capture processes race.
	return os.Link(file.Name(), path)
}
