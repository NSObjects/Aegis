package pullback

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
)

func replayFixture() PITSnapshot {
	day := time.Date(2026, 7, 6, 15, 0, 0, 0, shanghai)
	snapshot := PITSnapshot{
		StrategyVersion: StrategyVersion,
		CalendarSource:  "fixture:calendar",
		CalendarKnownAt: day.AddDate(0, 0, -1),
		UniverseSource:  "fixture:universe",
		UniverseKnownAt: day.AddDate(0, 0, -1),
		Members:         []PITUniverseEntry{{Symbol: "600519.SH"}},
	}
	for len(snapshot.Sessions) < 65 {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			date := day.Format("20060102")
			closePrice := 100 + float64(len(snapshot.Sessions))*0.1
			bar := technical.HistoricalBar{TradeDate: date, Open: closePrice, High: closePrice,
				Low: closePrice, Close: closePrice, Volume: 100, Amount: 100 * closePrice}
			factor, known := 1.0, day.Add(time.Hour)
			snapshot.Sessions = append(snapshot.Sessions, date)
			snapshot.Members[0].Rows = append(snapshot.Members[0].Rows, PITRow{
				Session: date, State: "TRADED", KnownAt: known, Bar: &bar, Factor: &factor, FactorKnownAt: &known,
			})
			snapshot.Cutoff = day.Add(2 * time.Hour)
		}
		day = day.AddDate(0, 0, 1)
	}
	return snapshot
}

func TestReplayPITUsesFrozenCalendarAndIsRepeatable(t *testing.T) {
	snapshot := replayFixture()
	first, err := ReplayPIT(snapshot)
	if err != nil || len(first.Results) != 1 || first.Results[0].Verdict != Selected {
		t.Fatalf("expected selected replay: %+v, %v", first, err)
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var reopened PITSnapshot
	if err := json.Unmarshal(raw, &reopened); err != nil {
		t.Fatal(err)
	}
	second, err := ReplayPIT(reopened)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("archived replay changed across encoding: %+v, %v", second, err)
	}
}

func TestReplayPITRejectsInputsUnavailableAtCutoff(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*PITSnapshot)
	}{
		{"future factor", func(s *PITSnapshot) { at := s.Cutoff.Add(time.Second); s.Members[0].Rows[0].FactorKnownAt = &at }},
		{"future bar", func(s *PITSnapshot) { s.Members[0].Rows[0].KnownAt = s.Cutoff.Add(time.Second) }},
		{"future universe", func(s *PITSnapshot) { s.UniverseKnownAt = s.Cutoff.Add(time.Second) }},
		{"unknown calendar", func(s *PITSnapshot) { s.CalendarKnownAt = time.Time{} }},
		{"incomplete session", func(s *PITSnapshot) { s.Cutoff = s.Cutoff.Add(-3 * time.Hour) }},
		{"non-session row", func(s *PITSnapshot) { s.Members[0].Rows[0].Session = "20990101" }},
		{"duplicate member", func(s *PITSnapshot) { s.Members = append(s.Members, s.Members[0]) }},
		{"invalid symbol", func(s *PITSnapshot) { s.Members[0].Symbol = "600xxx.SH" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := replayFixture()
			tc.change(&snapshot)
			if _, err := ReplayPIT(snapshot); err == nil {
				t.Fatal("replay accepted unverifiable inputs")
			}
		})
	}
}

func TestReplayPITRetainsHaltAndMissingEvidence(t *testing.T) {
	for _, tc := range []struct {
		state, reason string
	}{
		{"HALTED", "HALTED_SESSION_IN_WINDOW"},
		{"MISSING", "MISSING_TRADING_SESSION_DATA"},
		{"TRADED", "MISSING_PIT_ADJUSTMENT"},
	} {
		t.Run(tc.state, func(t *testing.T) {
			snapshot := replayFixture()
			row := &snapshot.Members[0].Rows[8]
			row.Factor, row.FactorKnownAt = nil, nil
			row.State = tc.state
			if tc.state != "TRADED" {
				row.Bar = nil
			}
			got, err := ReplayPIT(snapshot)
			if err != nil || len(got.Results) != 1 || got.Results[0].Verdict != InsufficientData || got.Results[0].Reason != tc.reason {
				t.Fatalf("missing evidence turned into a signal: %+v %v", got, err)
			}
		})
	}
}
