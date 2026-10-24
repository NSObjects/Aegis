package technical

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// BatchBar is one observed raw daily bar for one symbol from a market-wide
// batch query. Volume is in shares and amount in yuan, matching the units of
// HistoricalBar produced by the per-symbol researcher.
type BatchBar struct {
	Symbol    string
	TradeDate string
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	Amount    float64
}

// BatchMarketSource supplies point-in-time-safe, market-wide daily data for
// deterministic offline screening. It never invokes an LLM, never places
// orders, and never fabricates bars for missing sessions.
type BatchMarketSource interface {
	// TradingDays returns the most recent open exchange days at or before
	// asOf, ascending and deduplicated. Whether asOf itself is open is the
	// caller's decision: a non-trading asOf simply yields an earlier final day.
	TradingDays(ctx context.Context, asOf string, maxDays int) ([]string, error)
	// ListedSymbols returns symbols with list_status=L, unfiltered by market.
	ListedSymbols(ctx context.Context) ([]string, error)
	// DailyBars returns the raw daily bars observed on one trading date.
	// A symbol absent from the result simply has no bar on that date.
	DailyBars(ctx context.Context, tradeDate string) ([]BatchBar, error)
}

// TushareBatchSource implements BatchMarketSource over the Tushare HTTP API
// with trade-date batched queries: one calendar call, one listing call, and
// one call per trading day, instead of per-symbol request fan-out.
type TushareBatchSource struct{ api *TushareClient }

func NewTushareBatchSource(token string) (*TushareBatchSource, error) {
	client, err := NewTushareClient(token)
	if err != nil {
		return nil, err
	}
	return &TushareBatchSource{api: client}, nil
}

func (s *TushareBatchSource) TradingDays(ctx context.Context, asOf string, maxDays int) ([]string, error) {
	if s == nil || s.api == nil {
		return nil, errors.New("tushare batch source is nil")
	}
	if maxDays < 1 {
		return nil, errors.New("trading day count must be positive")
	}
	end, err := time.Parse("20060102", asOf)
	if err != nil {
		return nil, fmt.Errorf("invalid as-of date %q: %w", asOf, err)
	}
	// Two calendar days per requested trading day plus margin covers weekends
	// and multi-week exchange holidays without over-fetching the calendar.
	start := end.AddDate(0, 0, -(maxDays*2 + 10))
	// Request open days server-side and re-filter locally: the calendar is a
	// PIT boundary, so the client never trusts the provider's filtering alone.
	rows, err := s.api.query(ctx, "trade_cal", map[string]any{
		"exchange":    "SSE",
		"start_date":  start.Format("20060102"),
		"end_date":    asOf,
		"is_open":     "1",
	}, "cal_date", "is_open")
	if err != nil {
		return nil, err
	}
	days := make([]string, 0, len(rows))
	for _, row := range rows {
		day := str(row["cal_date"])
		if len(day) == 8 && str(row["is_open"]) == "1" {
			days = append(days, day)
		}
	}
	sort.Strings(days)
	if len(days) > maxDays {
		days = days[len(days)-maxDays:]
	}
	if len(days) == 0 {
		return nil, errors.New("trade calendar returned no open days")
	}
	return days, nil
}

func (s *TushareBatchSource) ListedSymbols(ctx context.Context) ([]string, error) {
	if s == nil || s.api == nil {
		return nil, errors.New("tushare batch source is nil")
	}
	// One page holds the whole listed A-share market; a full page means the
	// universe was silently truncated and must not be treated as complete.
	rows, err := s.api.query(ctx, "stock_basic", map[string]any{"list_status": "L"}, "ts_code")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("stock listing returned no listed symbols")
	}
	if len(rows) >= 6000 {
		return nil, errors.New("stock listing hit the single-page row limit")
	}
	symbols := make([]string, 0, len(rows))
	for _, row := range rows {
		code := str(row["ts_code"])
		if code != "" {
			symbols = append(symbols, code)
		}
	}
	if len(symbols) == 0 {
		return nil, errors.New("stock listing returned no usable symbols")
	}
	sort.Strings(symbols)
	return symbols, nil
}

func (s *TushareBatchSource) DailyBars(ctx context.Context, tradeDate string) ([]BatchBar, error) {
	if s == nil || s.api == nil {
		return nil, errors.New("tushare batch source is nil")
	}
	if len(tradeDate) != 8 || !isDigits(tradeDate) {
		return nil, fmt.Errorf("invalid trade date %q", tradeDate)
	}
	rows, err := s.api.query(ctx, "daily", map[string]any{"trade_date": tradeDate},
		"ts_code", "trade_date", "open", "high", "low", "close", "vol", "amount")
	if err != nil {
		return nil, err
	}
	bars := make([]BatchBar, 0, len(rows))
	for _, row := range rows {
		symbol := str(row["ts_code"])
		date := str(row["trade_date"])
		if symbol == "" || date != tradeDate {
			continue
		}
		// Tushare reports volume in lots and amount in thousands of yuan;
		// HistoricalBar uses shares and yuan.
		bar := BatchBar{
			Symbol:    symbol,
			TradeDate: date,
			Open:      num(row["open"]),
			High:      num(row["high"]),
			Low:       num(row["low"]),
			Close:     num(row["close"]),
			Volume:    num(row["vol"]) * 100,
			Amount:    num(row["amount"]) * 1000,
		}
		for _, v := range []float64{bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Amount} {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
				return nil, fmt.Errorf("invalid observed OHLCV for %s on %s", symbol, tradeDate)
			}
		}
		bars = append(bars, bar)
	}
	return bars, nil
}

func isDigits(value string) bool {
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(value) > 0
}
