package pullback

import (
	"reflect"
	"testing"
	"time"
)

func validationFixture(t *testing.T) ValidationInput {
	t.Helper()
	backtest := backtestFixture(t)
	for len(backtest.Sessions) < 7 {
		last := backtest.Sessions[len(backtest.Sessions)-1]
		date, err := sessionClose(last.Date)
		if err != nil {
			t.Fatal(err)
		}
		date = date.AddDate(0, 0, 1)
		for date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			date = date.AddDate(0, 0, 1)
		}
		row := last.Rows[0]
		row.KnownAt = date.Add(-5*time.Hour - 30*time.Minute)
		backtest.Sessions = append(backtest.Sessions, OpeningSession{Date: date.Format("20060102"), Rows: []OpeningRow{row}})
	}
	session := func(i int) string { return backtest.Sessions[i].Date }
	late := replayFixture()
	for i := 1; i <= 2; i++ {
		date, _ := sessionClose(session(i))
		last := late.Members[0].Rows[len(late.Members[0].Rows)-1]
		bar := *last.Bar
		bar.TradeDate = session(i)
		known := date.Add(time.Hour)
		late.Sessions = append(late.Sessions, session(i))
		late.Members[0].Rows = append(late.Members[0].Rows, PITRow{
			Session: session(i), State: "TRADED", KnownAt: known, Bar: &bar,
			Factor: last.Factor, FactorKnownAt: &known,
		})
		late.Cutoff = date.Add(2 * time.Hour)
	}
	backtest.Signals = append(backtest.Signals, late)
	entry, exit := &backtest.Sessions[3].Rows[0], &backtest.Sessions[4].Rows[0]
	entry.Open, exit.Open = 100, 95
	negative := false
	return ValidationInput{
		Backtest: backtest,
		Folds:    []ValidationFold{{TrainFrom: session(0), TrainThrough: session(1), TestFrom: session(2), TestThrough: session(4)}},
		Regimes:  []MarketRegime{{Date: session(2), Label: "盘整", KnownAt: late.Cutoff}},
		Outcomes: []CandidateOutcome{
			{Date: session(0), Symbol: "600519.SH", Positive: &negative, KnownAt: late.Cutoff, Source: "fixture:independent-label"},
			{Date: session(2), Symbol: "600519.SH", Positive: &negative, KnownAt: exit.KnownAt, Source: "fixture:independent-label"},
		},
	}
}

func TestValidateBacktestIsolatesWindowsAndReportsIndependentLabels(t *testing.T) {
	input := validationFixture(t)
	report, err := ValidateBacktest(input)
	if err != nil {
		t.Fatal(err)
	}
	train, test := report.Folds[0].InSample, report.Folds[0].OutOfSample
	if train.Metrics.Closed != 0 || train.Metrics.RealizedPnL != nil || train.Metrics.Classification != nil ||
		train.Metrics.TradeStatuses["OPEN"] != 1 || train.Regimes[0].Label != "UNCLASSIFIED" {
		t.Fatalf("later execution or outcome leaked into training: %+v", train)
	}
	got := test.Metrics
	if got.Closed != 1 || got.NegativeClosed != 1 || got.RealizedPnL == nil || *got.RealizedPnL >= 0 ||
		got.RealizedDrawdown == nil || *got.RealizedDrawdown != -*got.RealizedPnL || got.GrossTurnover <= 0 ||
		got.Classification == nil || got.Classification.Labeled != 1 || got.Classification.FalsePositive != 1 ||
		len(test.Regimes) != 1 || test.Regimes[0].Label != "盘整" || !reflect.DeepEqual(got, test.Regimes[0].Metrics) {
		t.Fatalf("unexpected evaluation: %+v", test)
	}
	again, err := ValidateBacktest(input)
	if err != nil || !reflect.DeepEqual(report, again) {
		t.Fatalf("evaluation was not reproducible: %+v, %v", again, err)
	}
}

func TestValidateBacktestFailsClosedOnLeakingMetadata(t *testing.T) {
	for _, change := range []func(*ValidationInput){
		func(in *ValidationInput) { in.Folds[0].TestFrom = in.Folds[0].TrainThrough },
		func(in *ValidationInput) { in.Regimes[0].KnownAt = in.Regimes[0].KnownAt.Add(time.Second) },
		func(in *ValidationInput) { in.Outcomes[0].Positive = nil },
		func(in *ValidationInput) { in.Folds = append(in.Folds, in.Folds[0]) },
	} {
		input := validationFixture(t)
		change(&input)
		if _, err := ValidateBacktest(input); err == nil {
			t.Fatal("invalid validation metadata was accepted")
		}
	}
}

func TestValidateBacktestAdvancesAcrossDisjointTestWindows(t *testing.T) {
	input := validationFixture(t)
	sessions := input.Backtest.Sessions
	later := input.Backtest.Signals[1]
	later.Members = append([]PITUniverseEntry(nil), later.Members...)
	for i := 3; i <= 5; i++ {
		session := sessions[i].Date
		closeAt, err := sessionClose(session)
		if err != nil {
			t.Fatal(err)
		}
		rows := later.Members[0].Rows
		last := rows[len(rows)-1]
		bar := *last.Bar
		bar.TradeDate = session
		known := closeAt.Add(time.Hour)
		later.Sessions = append(later.Sessions, session)
		later.Members[0].Rows = append(rows, PITRow{
			Session: session, State: "TRADED", KnownAt: known, Bar: &bar, Factor: last.Factor, FactorKnownAt: &known,
		})
		later.Cutoff = closeAt.Add(2 * time.Hour)
	}
	input.Backtest.Signals = append(input.Backtest.Signals, later)
	input.Folds = append(input.Folds, ValidationFold{
		TrainFrom: sessions[0].Date, TrainThrough: sessions[4].Date,
		TestFrom: sessions[5].Date, TestThrough: sessions[6].Date,
	})
	report, err := ValidateBacktest(input)
	if err != nil || len(report.Folds) != 2 {
		t.Fatalf("chronological walk-forward failed: %+v, %v", report, err)
	}
	next := report.Folds[1]
	if len(next.InSample.Replay.Signals) != 2 || len(next.OutOfSample.Replay.Signals) != 1 ||
		next.OutOfSample.Replay.Signals[0].AsOf != sessions[5].Date || next.OutOfSample.Metrics.Closed != 0 {
		t.Fatalf("rolling sample windows mixed signals or execution: %+v", next)
	}
}
