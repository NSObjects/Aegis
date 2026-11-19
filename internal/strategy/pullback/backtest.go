package pullback

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// BacktestInput combines independent PIT signal snapshots with archived opening
// execution data. The execution data is never used to evaluate the signal.
type BacktestInput struct {
	Policy   BacktestPolicy   `json:"policy"`
	Signals  []PITSnapshot    `json:"signals"`
	Sessions []OpeningSession `json:"sessions"`
}

type BacktestPolicy struct {
	Shares       int     `json:"shares"`
	HoldSessions int     `json:"hold_sessions"`
	BuyFeeBPS    float64 `json:"buy_fee_bps"`
	SellFeeBPS   float64 `json:"sell_fee_bps"`
	SlippageBPS  float64 `json:"slippage_bps"`
}

type OpeningSession struct {
	Date string       `json:"date"`
	Rows []OpeningRow `json:"rows"`
}

type OpeningRow struct {
	Symbol     string    `json:"symbol"`
	State      string    `json:"state"`
	KnownAt    time.Time `json:"known_at"`
	Open       float64   `json:"open,omitempty"`
	UpperLimit float64   `json:"upper_limit,omitempty"`
	LowerLimit float64   `json:"lower_limit,omitempty"`
	BuyShares  int       `json:"buy_shares,omitempty"`
	SellShares int       `json:"sell_shares,omitempty"`
	// Price continuity must be confirmed for each held session. An unresolved
	// split, dividend or other break makes realized P&L unavailable.
	ContinuousPrice bool `json:"continuous_price"`
}

type BacktestReport struct {
	Version string          `json:"simulation_version"`
	Policy  BacktestPolicy  `json:"policy"`
	Signals []PITReplay     `json:"signal_replays"`
	Trades  []BacktestTrade `json:"trades"`
}

type BacktestTrade struct {
	Symbol     string            `json:"symbol"`
	SignalDate string            `json:"signal_date"`
	Status     string            `json:"status"`
	Entry      *SimulatedFill    `json:"entry,omitempty"`
	Exit       *SimulatedFill    `json:"exit,omitempty"`
	NetPnL     *float64          `json:"net_pnl,omitempty"`
	Attempts   []BacktestAttempt `json:"attempts"`
}

type BacktestAttempt struct {
	Date   string `json:"date"`
	Side   string `json:"side"`
	Reason string `json:"reason"`
}

type SimulatedFill struct {
	Date     string  `json:"date"`
	Shares   int     `json:"shares"`
	Open     float64 `json:"open"`
	Price    float64 `json:"price"`
	Gross    float64 `json:"gross"`
	Fee      float64 `json:"fee"`
	Slippage float64 `json:"slippage"`
}

func Backtest(input BacktestInput) (BacktestReport, error) {
	policy := input.Policy
	if policy.Shares < 100 || policy.Shares > 1_000_000 || policy.Shares%100 != 0 ||
		policy.HoldSessions < 1 || policy.HoldSessions > 250 ||
		len(input.Signals) == 0 || len(input.Sessions) < 2 {
		return BacktestReport{}, errors.New("backtest requires a round lot, 1–250 holding sessions, signals and a calendar")
	}
	for _, bps := range []float64{policy.BuyFeeBPS, policy.SellFeeBPS, policy.SlippageBPS} {
		if math.IsNaN(bps) || bps < 0 || bps > 1000 {
			return BacktestReport{}, errors.New("fees and slippage must be between 0 and 1000 bps")
		}
	}
	sessions, positions, err := openingCalendar(input.Sessions)
	if err != nil {
		return BacktestReport{}, err
	}
	report := BacktestReport{
		Version: "opening-lot-v1", Policy: policy,
		Signals: make([]PITReplay, 0, len(input.Signals)), Trades: []BacktestTrade{},
	}
	dated := make(map[string]bool, len(input.Signals))
	for _, snapshot := range input.Signals {
		replay, err := ReplayPIT(snapshot)
		if err != nil {
			return BacktestReport{}, fmt.Errorf("signal replay: %w", err)
		}
		index, found := positions[replay.AsOf]
		if !found || dated[replay.AsOf] {
			return BacktestReport{}, errors.New("signal date must occur once in the simulation calendar")
		}
		signalIndex := len(snapshot.Sessions) - 1
		for i := range min(index, signalIndex) + 1 {
			if input.Sessions[index-i].Date != snapshot.Sessions[signalIndex-i] {
				return BacktestReport{}, errors.New("signal and execution calendars disagree")
			}
		}
		dated[replay.AsOf] = true
		report.Signals = append(report.Signals, replay)
	}
	sort.Slice(report.Signals, func(i, j int) bool { return report.Signals[i].AsOf < report.Signals[j].AsOf })
	heldUntil := make(map[string]int)
	for _, replay := range report.Signals {
		index := positions[replay.AsOf]
		for _, result := range replay.Results {
			if result.Verdict != Selected {
				continue
			}
			trade := BacktestTrade{Symbol: result.Symbol, SignalDate: replay.AsOf, Attempts: []BacktestAttempt{}}
			if end, exists := heldUntil[result.Symbol]; exists && end > index {
				trade.Status = "POSITION_ALREADY_OPEN"
			} else {
				end := simulateTrade(&trade, input.Sessions, sessions, index, policy)
				if trade.Entry != nil {
					heldUntil[result.Symbol] = end
				}
			}
			report.Trades = append(report.Trades, trade)
		}
	}
	return report, nil
}

func openingCalendar(sessions []OpeningSession) ([]map[string]OpeningRow, map[string]int, error) {
	rows := make([]map[string]OpeningRow, len(sessions))
	positions := make(map[string]int, len(sessions))
	for i, session := range sessions {
		closeAt, err := sessionClose(session.Date)
		if err != nil || (i > 0 && session.Date <= sessions[i-1].Date) {
			return nil, nil, errors.New("execution sessions must be valid, unique and ordered")
		}
		positions[session.Date] = i
		openAt := closeAt.Add(-5*time.Hour - 30*time.Minute)
		rows[i] = make(map[string]OpeningRow, len(session.Rows))
		for _, row := range session.Rows {
			if !validStockSymbol(row.Symbol) || rows[i][row.Symbol].State != "" || !knownAt(row.KnownAt, openAt) {
				return nil, nil, fmt.Errorf("invalid, duplicate or late opening row on %s", session.Date)
			}
			switch row.State {
			case "TRADED":
				if row.KnownAt.Before(openAt.Add(-5*time.Minute)) || !validOpeningPrice(row.LowerLimit) || !validOpeningPrice(row.UpperLimit) ||
					!validOpeningPrice(row.Open) || row.LowerLimit > row.Open || row.UpperLimit < row.Open ||
					row.LowerLimit >= row.UpperLimit || row.BuyShares < 0 || row.SellShares < 0 {
					return nil, nil, fmt.Errorf("invalid traded opening row on %s", session.Date)
				}
			case "HALTED", "MISSING":
				if row.Open != 0 || row.LowerLimit != 0 || row.UpperLimit != 0 || row.BuyShares != 0 || row.SellShares != 0 {
					return nil, nil, fmt.Errorf("non-traded opening row has execution data on %s", session.Date)
				}
			default:
				return nil, nil, fmt.Errorf("unknown opening state on %s", session.Date)
			}
			rows[i][row.Symbol] = row
		}
	}
	return rows, positions, nil
}

func validOpeningPrice(value float64) bool {
	return value > 0 && value <= 1_000_000 && math.Abs(value*100-math.Round(value*100)) < 0.000001
}

func cents(value float64) float64 { return math.Round(value*100) / 100 }

func attemptFill(date string, row OpeningRow, side string, policy BacktestPolicy) (*SimulatedFill, string) {
	switch row.State {
	case "TRADED":
	case "HALTED":
		return nil, "HALTED"
	default:
		return nil, "OPENING_DATA_MISSING"
	}
	shares, bps, direction := row.BuyShares, policy.BuyFeeBPS, 1.0
	// At the relevant price limit, opening liquidity on that side is not
	// assumed to fill a simulated order even when aggregate depth is supplied.
	if side == "BUY" && row.Open == row.UpperLimit {
		return nil, "UPPER_LIMIT"
	}
	if side == "SELL" {
		if row.Open == row.LowerLimit {
			return nil, "LOWER_LIMIT"
		}
		shares, bps, direction = row.SellShares, policy.SellFeeBPS, -1
	}
	if shares < policy.Shares {
		return nil, "INSUFFICIENT_OPENING_LIQUIDITY"
	}
	price := cents(row.Open * (1 + direction*policy.SlippageBPS/10_000))
	if price < row.LowerLimit || price > row.UpperLimit {
		return nil, "SLIPPAGE_OUTSIDE_PRICE_LIMIT"
	}
	gross := cents(price * float64(policy.Shares))
	return &SimulatedFill{
		Date: date, Shares: policy.Shares, Open: row.Open, Price: price, Gross: gross,
		Fee: cents(gross * bps / 10_000), Slippage: cents(math.Abs(price-row.Open) * float64(policy.Shares)),
	}, ""
}

func simulateTrade(trade *BacktestTrade, calendar []OpeningSession, rows []map[string]OpeningRow, signal int, policy BacktestPolicy) int {
	entryIndex := signal + 1
	if entryIndex >= len(calendar) {
		trade.Status = "NO_ENTRY_SESSION"
		return signal
	}
	entryDate := calendar[entryIndex].Date
	entry, reason := attemptFill(entryDate, rows[entryIndex][trade.Symbol], "BUY", policy)
	if reason != "" {
		trade.Status = "ENTRY_UNFILLED"
		trade.Attempts = append(trade.Attempts, BacktestAttempt{entryDate, "BUY", reason})
		return signal
	}
	trade.Entry = entry
	trade.Status = "OPEN"
	for i := entryIndex + 1; i < len(calendar); i++ {
		row := rows[i][trade.Symbol]
		date := calendar[i].Date
		if !row.ContinuousPrice {
			trade.Status = "VALUATION_UNAVAILABLE"
			trade.Attempts = append(trade.Attempts, BacktestAttempt{date, "HOLD", "PRICE_CONTINUITY_UNVERIFIED"})
			break
		}
		if i-entryIndex < policy.HoldSessions {
			continue
		}
		exit, reason := attemptFill(date, row, "SELL", policy)
		if reason != "" {
			trade.Attempts = append(trade.Attempts, BacktestAttempt{date, "SELL", reason})
			continue
		}
		trade.Exit = exit
		trade.Status = "CLOSED"
		net := cents(exit.Gross - exit.Fee - entry.Gross - entry.Fee)
		trade.NetPnL = &net
		return i
	}
	return len(calendar)
}
