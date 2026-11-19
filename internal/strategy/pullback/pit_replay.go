package pullback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"Aegis/internal/capability/technical"
)

// PITSnapshot describes one frozen cutoff. All timestamps are evidence of
// source availability supplied by the archive, not the import time.
type PITSnapshot struct {
	StrategyVersion string             `json:"strategy_version"`
	Cutoff          time.Time          `json:"cutoff"`
	CalendarSource  string             `json:"calendar_source"`
	CalendarKnownAt time.Time          `json:"calendar_known_at"`
	Sessions        []string           `json:"sessions"`
	UniverseSource  string             `json:"universe_source"`
	UniverseKnownAt time.Time          `json:"universe_known_at"`
	Members         []PITUniverseEntry `json:"members"`
}

type PITUniverseEntry struct {
	Symbol string   `json:"symbol"`
	Rows   []PITRow `json:"rows"`
}

type PITRow struct {
	Session       string                   `json:"session"`
	State         string                   `json:"state"`
	KnownAt       time.Time                `json:"known_at"`
	Bar           *technical.HistoricalBar `json:"bar,omitempty"`
	Factor        *float64                 `json:"factor,omitempty"`
	FactorKnownAt *time.Time               `json:"factor_known_at,omitempty"`
}

type PITReplay struct {
	AsOf             string            `json:"as_of"`
	SnapshotSHA256   string            `json:"snapshot_sha256"`
	DefinitionSHA256 string            `json:"strategy_definition_sha256"`
	Definition       StrategyCandidate `json:"strategy_definition"`
	Results          []Result          `json:"results"`
}

var shanghai = time.FixedZone("Asia/Shanghai", 8*60*60)

func sessionClose(session string) (time.Time, error) {
	day, err := time.ParseInLocation("20060102", session, shanghai)
	if err != nil || day.Format("20060102") != session {
		return time.Time{}, errors.New("invalid trading session")
	}
	return day.Add(15 * time.Hour), nil
}

func knownAt(at, cutoff time.Time) bool {
	return !at.IsZero() && !at.After(cutoff)
}

// ReplayPIT uses only archived inputs. It verifies their time constraints and
// session alignment, without inferring the authenticity or coverage of sources.
func ReplayPIT(snapshot PITSnapshot) (PITReplay, error) {
	definition, err := Definition(snapshot.StrategyVersion)
	if err != nil {
		return PITReplay{}, err
	}
	if snapshot.Cutoff.IsZero() || strings.TrimSpace(snapshot.CalendarSource) == "" || strings.TrimSpace(snapshot.UniverseSource) == "" ||
		!knownAt(snapshot.CalendarKnownAt, snapshot.Cutoff) || !knownAt(snapshot.UniverseKnownAt, snapshot.Cutoff) ||
		len(snapshot.Sessions) == 0 || len(snapshot.Members) == 0 {
		return PITReplay{}, errors.New("cutoff, known calendar and frozen universe are required")
	}
	asOf := snapshot.Cutoff.In(shanghai).Format("20060102")
	if snapshot.Sessions[len(snapshot.Sessions)-1] != asOf {
		return PITReplay{}, errors.New("calendar must end at cutoff trading date")
	}
	sessions := make(map[string]time.Time, len(snapshot.Sessions))
	previous := ""
	for _, date := range snapshot.Sessions {
		closeAt, err := sessionClose(date)
		if err != nil || date <= previous || closeAt.After(snapshot.Cutoff) {
			return PITReplay{}, errors.New("calendar contains an unordered, incomplete or future trading session")
		}
		sessions[date] = closeAt
		previous = date
	}

	results := make([]Result, 0, len(snapshot.Members))
	seen := make(map[string]bool, len(snapshot.Members))
	for _, member := range snapshot.Members {
		if seen[member.Symbol] || !validStockSymbol(member.Symbol) || !technical.IsMainBoardSymbol(member.Symbol) {
			return PITReplay{}, fmt.Errorf("invalid or duplicate universe member %q", member.Symbol)
		}
		seen[member.Symbol] = true
		observation, reason, err := pitObservation(snapshot, member, sessions, definition)
		if err != nil {
			return PITReplay{}, fmt.Errorf("%s: %w", member.Symbol, err)
		}
		if reason != "" {
			results = append(results, Result{Symbol: member.Symbol, AsOf: asOf, Version: definition.Version, Verdict: InsufficientData, Reason: reason})
			continue
		}
		replayed, err := ScanVersion(definition.Version, asOf, []technical.HistoricalObservation{observation})
		if err != nil {
			return PITReplay{}, fmt.Errorf("%s: %w", member.Symbol, err)
		}
		results = append(results, replayed[0])
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Symbol < results[j].Symbol })
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return PITReplay{}, err
	}
	sum := sha256.Sum256(raw)
	digest, err := definition.SHA256()
	if err != nil {
		return PITReplay{}, err
	}
	return PITReplay{AsOf: asOf, SnapshotSHA256: hex.EncodeToString(sum[:]), DefinitionSHA256: digest, Definition: definition, Results: results}, nil
}

func pitObservation(snapshot PITSnapshot, member PITUniverseEntry, sessions map[string]time.Time, definition StrategyCandidate) (technical.HistoricalObservation, string, error) {
	rows := make(map[string]PITRow, len(member.Rows))
	for _, row := range member.Rows {
		closeAt, exists := sessions[row.Session]
		if !exists || rows[row.Session].State != "" || !knownAt(row.KnownAt, snapshot.Cutoff) {
			return technical.HistoricalObservation{}, "", errors.New("duplicate, unavailable or non-session row")
		}
		switch row.State {
		case "TRADED":
			if row.Bar == nil || row.Bar.TradeDate != row.Session || row.KnownAt.Before(closeAt) || row.Bar.Volume <= 0 {
				return technical.HistoricalObservation{}, "", errors.New("invalid traded row")
			}
			if row.Factor != nil {
				if *row.Factor <= 0 || math.IsNaN(*row.Factor) || math.IsInf(*row.Factor, 0) ||
					row.FactorKnownAt == nil || !knownAt(*row.FactorKnownAt, snapshot.Cutoff) {
					return technical.HistoricalObservation{}, "", errors.New("invalid or unavailable adjustment factor")
				}
			} else if row.FactorKnownAt != nil {
				return technical.HistoricalObservation{}, "", errors.New("factor availability without a value")
			}
		case "HALTED", "MISSING":
			if row.Bar != nil || row.Factor != nil || row.FactorKnownAt != nil {
				return technical.HistoricalObservation{}, "", errors.New("non-traded row contains prices or factors")
			}
		default:
			return technical.HistoricalObservation{}, "", errors.New("unknown session state")
		}
		rows[row.Session] = row
	}
	asOf := snapshot.Sessions[len(snapshot.Sessions)-1]
	observation := technical.HistoricalObservation{Symbol: member.Symbol, RequestedAsOf: asOf, EffectiveAsOf: asOf, PriceBasis: definition.PriceBasis}
	window := definition.SlowSessions + definition.SlopeLagSessions
	if len(snapshot.Sessions) < window {
		return observation, "MISSING_CALENDAR_WINDOW", nil
	}
	for _, session := range snapshot.Sessions[len(snapshot.Sessions)-window:] {
		row := rows[session]
		switch row.State {
		case "HALTED":
			return observation, "HALTED_SESSION_IN_WINDOW", nil
		case "TRADED":
			if row.Factor == nil {
				return observation, "MISSING_PIT_ADJUSTMENT", nil
			}
			observation.Bars = append(observation.Bars, *row.Bar)
			observation.PITFactors = append(observation.PITFactors, technical.PITAdjustmentFactor{TradeDate: session, Factor: *row.Factor})
		default:
			return observation, "MISSING_TRADING_SESSION_DATA", nil
		}
	}
	return observation, "", nil
}
