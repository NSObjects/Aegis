package pullback

import (
	"reflect"
	"testing"
	"time"
)

func backtestFixture(t *testing.T) BacktestInput {
	t.Helper()
	signal := replayFixture()
	date := signal.Sessions[len(signal.Sessions)-1]
	day, err := sessionClose(date)
	if err != nil {
		t.Fatal(err)
	}
	input := BacktestInput{
		Policy:   BacktestPolicy{Shares: 100, HoldSessions: 1, BuyFeeBPS: 10, SellFeeBPS: 20, SlippageBPS: 10},
		Signals:  []PITSnapshot{signal},
		Sessions: []OpeningSession{{Date: date}},
	}
	for len(input.Sessions) < 4 {
		day = day.AddDate(0, 0, 1)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		price := 100.0
		if len(input.Sessions) >= 2 {
			price = 110
		}
		input.Sessions = append(input.Sessions, OpeningSession{
			Date: day.Format("20060102"),
			Rows: []OpeningRow{{
				Symbol: "600519.SH", State: "TRADED", KnownAt: day.Add(-5*time.Hour - 30*time.Minute),
				Open: price, LowerLimit: 90, UpperLimit: 120, BuyShares: 1000, SellShares: 1000, ContinuousPrice: true,
			}},
		})
	}
	return input
}

func TestBacktestChargesCostsAndDefersLimitBlockedExit(t *testing.T) {
	input := backtestFixture(t)
	firstExit := &input.Sessions[2].Rows[0]
	firstExit.Open = firstExit.LowerLimit
	got, err := Backtest(input)
	if err != nil || len(got.Trades) != 1 {
		t.Fatalf("backtest failed: %+v, %v", got, err)
	}
	trade := got.Trades[0]
	if trade.Status != "CLOSED" || trade.Entry == nil || trade.Exit == nil || trade.NetPnL == nil {
		t.Fatalf("expected a closed trade: %+v", trade)
	}
	if trade.Entry.Date != input.Sessions[1].Date || trade.Exit.Date != input.Sessions[3].Date ||
		len(trade.Attempts) != 1 || trade.Attempts[0].Reason != "LOWER_LIMIT" {
		t.Fatalf("next-open, T+1 or price limit violated: %+v", trade)
	}
	if trade.Entry.Gross != 10010 || trade.Entry.Fee != 10.01 || trade.Entry.Slippage != 10 ||
		trade.Exit.Gross != 10989 || trade.Exit.Fee != 21.98 || trade.Exit.Slippage != 11 || *trade.NetPnL != 947.01 {
		t.Fatalf("fees, slippage or net P&L incorrect: %+v, %+v, %f", trade.Entry, trade.Exit, *trade.NetPnL)
	}
	again, err := Backtest(input)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatalf("same archive and policy must replay deterministically: %+v, %v", again, err)
	}
}

func TestBacktestRefusesUnfillableEntriesAndUnknownValuations(t *testing.T) {
	for _, tc := range []struct {
		name, status, reason string
		change               func(*BacktestInput)
	}{
		{"upper limit", "ENTRY_UNFILLED", "UPPER_LIMIT", func(in *BacktestInput) {
			row := &in.Sessions[1].Rows[0]
			row.Open = row.UpperLimit
		}},
		{"liquidity", "ENTRY_UNFILLED", "INSUFFICIENT_OPENING_LIQUIDITY", func(in *BacktestInput) {
			in.Sessions[1].Rows[0].BuyShares = 99
		}},
		{"halt", "ENTRY_UNFILLED", "HALTED", func(in *BacktestInput) {
			row := in.Sessions[1].Rows[0]
			in.Sessions[1].Rows[0] = OpeningRow{Symbol: row.Symbol, State: "HALTED", KnownAt: row.KnownAt}
		}},
		{"corporate action", "VALUATION_UNAVAILABLE", "PRICE_CONTINUITY_UNVERIFIED", func(in *BacktestInput) {
			in.Sessions[2].Rows[0].ContinuousPrice = false
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := backtestFixture(t)
			tc.change(&input)
			report, err := Backtest(input)
			if err != nil || len(report.Trades) != 1 {
				t.Fatalf("unexpected report: %+v, %v", report, err)
			}
			trade := report.Trades[0]
			if trade.NetPnL != nil || trade.Exit != nil || trade.Status != tc.status || len(trade.Attempts) != 1 || trade.Attempts[0].Reason != tc.reason {
				t.Fatalf("unverifiable entry or outcome misreported: %+v", trade)
			}
		})
	}
}

func TestBacktestRejectsLateOpeningDataAndZeroSessionHolding(t *testing.T) {
	for _, change := range []func(*BacktestInput){
		func(in *BacktestInput) {
			in.Sessions[1].Rows[0].KnownAt = in.Sessions[1].Rows[0].KnownAt.Add(time.Second)
		},
		func(in *BacktestInput) { in.Policy.HoldSessions = 0 },
	} {
		input := backtestFixture(t)
		change(&input)
		if _, err := Backtest(input); err == nil {
			t.Fatal("invalid backtest passed validation")
		}
	}
}
