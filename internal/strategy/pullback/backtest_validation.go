package pullback

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ValidationInput freezes the simulation policy for every window. The report
// measures forward performance and does not select or approve policies.
type ValidationInput struct {
	Backtest BacktestInput      `json:"backtest"`
	Folds    []ValidationFold   `json:"folds"`
	Regimes  []MarketRegime     `json:"regimes,omitempty"`
	Outcomes []CandidateOutcome `json:"candidate_outcomes,omitempty"`
}

type ValidationFold struct {
	TrainFrom    string `json:"train_from"`
	TrainThrough string `json:"train_through"`
	TestFrom     string `json:"test_from"`
	TestThrough  string `json:"test_through"`
}

type MarketRegime struct {
	Date    string    `json:"date"`
	Label   string    `json:"label"`
	KnownAt time.Time `json:"known_at"`
}

// CandidateOutcome is an independently sourced classification label.
// Its time is filtered at each fold boundary, including training windows.
type CandidateOutcome struct {
	Date     string    `json:"date"`
	Symbol   string    `json:"symbol"`
	Positive *bool     `json:"positive"`
	KnownAt  time.Time `json:"known_at"`
	Source   string    `json:"source"`
}

type ClassificationMetrics struct {
	Labeled       int     `json:"labeled_candidates"`
	TruePositive  int     `json:"true_positive"`
	TrueNegative  int     `json:"true_negative"`
	FalsePositive int     `json:"false_positive"`
	FalseNegative int     `json:"false_negative"`
	ErrorRate     float64 `json:"error_rate"`
}

type ValidationReport struct {
	Version string           `json:"validation_version"`
	Policy  BacktestPolicy   `json:"policy"`
	Folds   []FoldEvaluation `json:"folds"`
}

type FoldEvaluation struct {
	Range       ValidationFold `json:"range"`
	InSample    WindowReport   `json:"in_sample"`
	OutOfSample WindowReport   `json:"out_of_sample"`
}

type WindowReport struct {
	Metrics           EvaluationMetrics  `json:"metrics"`
	Regimes           []RegimeEvaluation `json:"regimes"`
	Replay            BacktestReport     `json:"replay"`
	CandidateOutcomes []CandidateOutcome `json:"candidate_outcomes,omitempty"`
}

type RegimeEvaluation struct {
	Label   string            `json:"label"`
	Metrics EvaluationMetrics `json:"metrics"`
}

type EvaluationMetrics struct {
	Candidates         int                    `json:"evaluated_candidates"`
	MissingCandidates  int                    `json:"insufficient_candidates"`
	Selected           int                    `json:"selected_signals"`
	Closed             int                    `json:"closed_trades"`
	NegativeClosed     int                    `json:"negative_closed_trades"`
	NegativeClosedRate *float64               `json:"negative_closed_trade_rate,omitempty"`
	RealizedPnL        *float64               `json:"realized_pnl,omitempty"`
	ClosedCostReturn   *float64               `json:"closed_cost_return,omitempty"`
	RealizedDrawdown   *float64               `json:"realized_pnl_max_drawdown,omitempty"`
	GrossTurnover      float64                `json:"gross_turnover"`
	TradeStatuses      map[string]int         `json:"trade_statuses"`
	Classification     *ClassificationMetrics `json:"classification,omitempty"`
}

func ValidateBacktest(input ValidationInput) (ValidationReport, error) {
	if len(input.Folds) == 0 || len(input.Folds) > 50 {
		return ValidationReport{}, errors.New("provide 1–50 chronological evaluation folds")
	}
	_, indices, err := openingCalendar(input.Backtest.Sessions)
	if err != nil {
		return ValidationReport{}, err
	}
	cutoffs := make(map[string]time.Time, len(input.Backtest.Signals))
	version := ""
	for _, snapshot := range input.Backtest.Signals {
		date := snapshot.Cutoff.In(shanghai).Format("20060102")
		if version != "" && version != snapshot.StrategyVersion {
			return ValidationReport{}, errors.New("evaluation must use one strategy version")
		}
		version = snapshot.StrategyVersion
		if _, found := indices[date]; snapshot.Cutoff.IsZero() || !found || !cutoffs[date].IsZero() {
			return ValidationReport{}, errors.New("each signal cutoff must appear once in the execution calendar")
		}
		cutoffs[date] = snapshot.Cutoff
	}
	regimes := make(map[string]string, len(input.Regimes))
	for _, regime := range input.Regimes {
		if cutoffs[regime.Date].IsZero() || !knownAt(regime.KnownAt, cutoffs[regime.Date]) ||
			regimes[regime.Date] != "" || strings.TrimSpace(regime.Label) != regime.Label ||
			regime.Label == "" || regime.Label == "UNCLASSIFIED" || len(regime.Label) > 64 {
			return ValidationReport{}, errors.New("invalid or late market regime label")
		}
		regimes[regime.Date] = regime.Label
	}
	outcomes := make(map[[2]string]CandidateOutcome, len(input.Outcomes))
	for _, outcome := range input.Outcomes {
		key := [2]string{outcome.Date, outcome.Symbol}
		cutoff := cutoffs[outcome.Date]
		if cutoff.IsZero() || !outcome.KnownAt.After(cutoff) || !validStockSymbol(outcome.Symbol) ||
			outcomes[key].Positive != nil || outcome.Positive == nil || strings.TrimSpace(outcome.Source) == "" || len(outcome.Source) > 512 {
			return ValidationReport{}, errors.New("invalid or duplicate candidate outcome label")
		}
		outcomes[key] = outcome
	}
	report := ValidationReport{
		Version: "fixed-policy-walk-forward-v1", Policy: input.Backtest.Policy,
		Folds: make([]FoldEvaluation, 0, len(input.Folds)),
	}
	previousTestThrough := ""
	previousTrainThrough := ""
	for i, fold := range input.Folds {
		if fold.TrainFrom >= fold.TrainThrough || fold.TrainThrough >= fold.TestFrom ||
			fold.TestFrom >= fold.TestThrough || fold.TestFrom <= previousTestThrough ||
			fold.TrainThrough <= previousTrainThrough {
			return ValidationReport{}, fmt.Errorf("fold %d: training must precede testing and test windows must not overlap", i+1)
		}
		train, err := evaluateWindow(input.Backtest, indices, regimes, outcomes, fold.TrainFrom, fold.TrainThrough)
		if err != nil {
			return ValidationReport{}, fmt.Errorf("fold %d in-sample: %w", i+1, err)
		}
		test, err := evaluateWindow(input.Backtest, indices, regimes, outcomes, fold.TestFrom, fold.TestThrough)
		if err != nil {
			return ValidationReport{}, fmt.Errorf("fold %d out-of-sample: %w", i+1, err)
		}
		report.Folds = append(report.Folds, FoldEvaluation{Range: fold, InSample: train, OutOfSample: test})
		previousTestThrough, previousTrainThrough = fold.TestThrough, fold.TrainThrough
	}
	return report, nil
}

func evaluateWindow(input BacktestInput, indices map[string]int, regimes map[string]string, outcomes map[[2]string]CandidateOutcome, from, through string) (WindowReport, error) {
	start, startOK := indices[from]
	end, endOK := indices[through]
	if !startOK || !endOK || start >= end {
		return WindowReport{}, errors.New("window boundaries must be trading sessions")
	}
	window := BacktestInput{Policy: input.Policy, Sessions: input.Sessions[start : end+1]}
	for _, snapshot := range input.Signals {
		date := snapshot.Cutoff.In(shanghai).Format("20060102")
		if date >= from && date <= through {
			window.Signals = append(window.Signals, snapshot)
		}
	}
	if len(window.Signals) == 0 {
		return WindowReport{}, errors.New("window contains no signal snapshots")
	}
	replayed, err := Backtest(window)
	if err != nil {
		return WindowReport{}, err
	}
	byRegime := make(map[string]*BacktestReport)
	labels := make(map[string]string, len(replayed.Signals))
	for _, signal := range replayed.Signals {
		label := regimes[signal.AsOf]
		if label == "" {
			label = "UNCLASSIFIED"
		}
		labels[signal.AsOf] = label
		if byRegime[label] == nil {
			byRegime[label] = &BacktestReport{}
		}
		byRegime[label].Signals = append(byRegime[label].Signals, signal)
	}
	for _, trade := range replayed.Trades {
		group := byRegime[labels[trade.SignalDate]]
		group.Trades = append(group.Trades, trade)
	}
	usedOutcomes := make(map[[2]string]CandidateOutcome)
	labelsUsed := make([]CandidateOutcome, 0)
	for _, signal := range replayed.Signals {
		for _, candidate := range signal.Results {
			key := [2]string{signal.AsOf, candidate.Symbol}
			outcome, found := outcomes[key]
			if found && candidate.Verdict != InsufficientData && outcome.KnownAt.In(shanghai).Format("20060102") <= through {
				usedOutcomes[key] = outcome
				labelsUsed = append(labelsUsed, outcome)
			}
		}
	}
	report := WindowReport{
		Metrics: summarizeWindow(replayed, usedOutcomes), Replay: replayed, CandidateOutcomes: labelsUsed,
		Regimes: make([]RegimeEvaluation, 0, len(byRegime)),
	}
	for label, group := range byRegime {
		report.Regimes = append(report.Regimes, RegimeEvaluation{Label: label, Metrics: summarizeWindow(*group, usedOutcomes)})
	}
	sort.Slice(report.Regimes, func(i, j int) bool { return report.Regimes[i].Label < report.Regimes[j].Label })
	return report, nil
}

func summarizeWindow(replayed BacktestReport, outcomes map[[2]string]CandidateOutcome) EvaluationMetrics {
	classification := ClassificationMetrics{}
	result := EvaluationMetrics{TradeStatuses: make(map[string]int)}
	for _, replay := range replayed.Signals {
		for _, candidate := range replay.Results {
			result.Candidates++
			outcome, found := outcomes[[2]string{replay.AsOf, candidate.Symbol}]
			if found {
				classification.Labeled++
				switch {
				case candidate.Verdict == Selected && *outcome.Positive:
					classification.TruePositive++
				case candidate.Verdict == Selected:
					classification.FalsePositive++
				case *outcome.Positive:
					classification.FalseNegative++
				default:
					classification.TrueNegative++
				}
			}
			switch candidate.Verdict {
			case InsufficientData:
				result.MissingCandidates++
			case Selected:
				result.Selected++
			}
		}
	}
	if classification.Labeled > 0 {
		classification.ErrorRate = float64(classification.FalsePositive+classification.FalseNegative) / float64(classification.Labeled)
		result.Classification = &classification
	}
	pnlByDate := make(map[string]float64)
	var closedCost float64
	for _, trade := range replayed.Trades {
		result.TradeStatuses[trade.Status]++
		if trade.Entry != nil {
			result.GrossTurnover += trade.Entry.Gross
		}
		if trade.Exit != nil {
			result.GrossTurnover += trade.Exit.Gross
		}
		if trade.Status != "CLOSED" || trade.NetPnL == nil {
			continue
		}
		result.Closed++
		if *trade.NetPnL < 0 {
			result.NegativeClosed++
		}
		closedCost += trade.Entry.Gross + trade.Entry.Fee
		pnlByDate[trade.Exit.Date] += *trade.NetPnL
	}
	result.GrossTurnover = cents(result.GrossTurnover)
	if result.Closed == 0 {
		return result
	}
	dates := make([]string, 0, len(pnlByDate))
	for date := range pnlByDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	var equity, peak, drawdown float64
	for _, date := range dates {
		equity = cents(equity + pnlByDate[date])
		peak = math.Max(peak, equity)
		drawdown = math.Max(drawdown, cents(peak-equity))
	}
	loserRate := float64(result.NegativeClosed) / float64(result.Closed)
	netReturn := equity / closedCost
	result.RealizedPnL, result.RealizedDrawdown = &equity, &drawdown
	result.ClosedCostReturn, result.NegativeClosedRate = &netReturn, &loserRate
	return result
}
