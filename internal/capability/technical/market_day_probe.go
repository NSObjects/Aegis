package technical

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// MarketDayProbe validates a bounded real-provider sample without creating a
// scan, triggering research, or modifying the production market-day cache.
// Coverage is measured against PIT main-board membership at the selected
// cutoff; it cannot establish source completeness or historical revisions.
type MarketDayProbe struct {
	AsOf               string               `json:"as_of"`
	UniverseSource     string               `json:"universe_source"`
	MainBoardSymbols   int                  `json:"main_board_symbols"`
	TradingDaysSampled int                  `json:"trading_days_sampled"`
	MaxProviderCalls   int                  `json:"max_provider_calls"`
	Days               []MarketDayProbeDate `json:"days"`
	ReadyForFullPool   bool                 `json:"ready_for_full_pool"`
	Warnings           []string             `json:"warnings"`
}

type MarketDayProbeDate struct {
	Date              string `json:"date"`
	DailyRows         int    `json:"daily_rows"`
	FactorRows        int    `json:"factor_rows"`
	MainBoardDaily    int    `json:"main_board_daily"`
	MainBoardFactors  int    `json:"main_board_factors"`
	PairedRows        int    `json:"paired_rows"`
	MainBoardCount    int    `json:"main_board_count"`
	DailyBytes        int64  `json:"daily_snapshot_bytes"`
	FactorBytes       int64  `json:"factor_snapshot_bytes"`
	SevereUndercover  bool   `json:"severe_undercoverage"`
}

// Probe uses 3 stock_basic listing partitions + 1 SSE trade_cal query +
// exactly 2 market-date reads per day when called with a fresh temporary
// cache directory. Every date uses the same checked full-market cache reader
// as production (including provider-cap, checksum, and data validation).
// It never writes to a production store unless the caller incorrectly
// supplies that directory; the CLI always creates a private scratch folder.
func (c *MarketDayCache) Probe(ctx context.Context, asOf string, days int) (MarketDayProbe, error) {
	report := MarketDayProbe{
		AsOf: asOf,
		TradingDaysSampled: days,
		MaxProviderCalls: 4 + 2*days,
		Days: []MarketDayProbeDate{},
		Warnings: []string{},
	}
	if c == nil || c.api == nil || ctx == nil {
		return report, errors.New("market day probe requires an initialized cache and context")
	}
	if days < 1 || days > 3 || !withinPoolWindow(asOf, asOf, asOf) {
		return report, errors.New("market day probe accepts only 1..3 days and a valid YYYYMMDD cutoff")
	}
	// Freeze listing membership once, following production's PIT universe
	// contract. Do not issue stock_basic per sampled date.
	universe, err := c.api.ListStockUniverse(ctx, asOf)
	if err != nil {
		return report, fmt.Errorf("canary PIT universe unavailable: %w", err)
	}
	sessions, err := c.tradingSessions(ctx, asOf)
	if err != nil {
		return report, fmt.Errorf("canary SSE calendar unavailable: %w", err)
	}
	report.UniverseSource = universe.Source
	report.MainBoardSymbols = len(universe.Symbols)
	membership := make(map[string]struct{}, len(universe.Symbols))
	for _, symbol := range universe.Symbols {
		membership[symbol] = struct{}{}
	}
	report.ReadyForFullPool = true
	for _, date := range sessions[len(sessions)-days:] {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		daily, err := c.readDate(ctx, "daily", date)
		if err != nil {
			return report, fmt.Errorf("canary daily %s rejected: %w", date, err)
		}
		factors, err := c.readDate(ctx, "adj_factor", date)
		if err != nil {
			return report, fmt.Errorf("canary adjustment factors %s rejected: %w", date, err)
		}
		snapshot := MarketDayProbeDate{
			Date: date,
			DailyRows: len(daily.Prices),
			FactorRows: len(factors.Factors),
			MainBoardCount: len(membership),
		}
		factorSymbols := make(map[string]struct{}, len(factors.Factors))
		for _, row := range factors.Factors {
			if _, ok := membership[row.Symbol]; !ok {
				continue
			}
			snapshot.MainBoardFactors++
			factorSymbols[row.Symbol] = struct{}{}
		}
		for _, row := range daily.Prices {
			if _, ok := membership[row.Symbol]; !ok {
				continue
			}
			snapshot.MainBoardDaily++
			if _, ok := factorSymbols[row.Symbol]; ok {
				snapshot.PairedRows++
			}
		}
		dailyStat, err := os.Stat(filepath.Join(c.dir, "daily_"+date+"_v1.json"))
		if err != nil {
			return report, fmt.Errorf("cannot stat canary daily snapshot: %w", err)
		}
		factorStat, err := os.Stat(filepath.Join(c.dir, "adj_factor_"+date+"_v1.json"))
		if err != nil {
			return report, fmt.Errorf("cannot stat canary adjustment snapshot: %w", err)
		}
		snapshot.DailyBytes = dailyStat.Size()
		snapshot.FactorBytes = factorStat.Size()
		// Identical conservative floor to production's large frozen pool
		// check; a passing rate is a signal, not a completeness proof.
		if len(membership) >= 100 &&
			(snapshot.MainBoardDaily*100 < len(membership)*80 ||
				snapshot.MainBoardFactors*100 < len(membership)*90) {
			snapshot.SevereUndercover = true
			report.ReadyForFullPool = false
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("%s: severe market coverage gap (daily %d/%d, factors %d/%d)",
					date, snapshot.MainBoardDaily, len(membership),
					snapshot.MainBoardFactors, len(membership)))
		}
		if snapshot.PairedRows < snapshot.MainBoardDaily {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("%s: %d main-board daily rows lack a same-date factor",
					date, snapshot.MainBoardDaily-snapshot.PairedRows))
		}
		report.Days = append(report.Days, snapshot)
	}
	report.Warnings = append(report.Warnings,
		"Passing this limited sample does not prove full 90-day coverage, provider revisions, or historical point-in-time factor availability")
	return report, nil
}
