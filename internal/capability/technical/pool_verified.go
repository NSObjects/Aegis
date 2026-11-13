package technical

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// verifiedPoolCutoffKey is intentionally private. Only the in-process
// PoolManager, after its PIT universe and trading calendar were validated,
// can mark an entire batch as eligible for the two-query scan path.
// This is not a client-supplied request field.
type verifiedPoolCutoffKey struct{}

// WithVerifiedPoolCutoff marks only a *previously validated* market-universe
// scan. Callers must first check trading day, membership and frozen as-of.
// Ordinary symbol research must never use this shortcut.
func WithVerifiedPoolCutoff(ctx context.Context, asOf string) context.Context {
	return context.WithValue(ctx, verifiedPoolCutoffKey{}, asOf)
}

func (r *HTTPResearcher) researchVerifiedPool(ctx context.Context, symbol, asOf string, asOfDate time.Time) (map[string]any, error) {
	// Historical-only read: a frozen pool has already checked stock_basic
	// membership and the SSE trading calendar for its exact cutoff. Unlike
	// generic Research, this does not require daily_basic or another
	// per-symbol request just to confirm the target close.
	from := asOfDate.AddDate(0, 0, -420).Format("20060102")
	daily, err := r.api.query(ctx, "daily",
		map[string]any{"ts_code": symbol, "start_date": from, "end_date": asOf},
		"ts_code", "trade_date", "open", "high", "low", "close", "pre_close", "pct_chg", "vol", "amount")
	if err != nil {
		return nil, err
	}
	if len(daily) == 0 {
		return statusMap(symbol, asOf, "NO_DATA", "", "no historical bars at or before the verified cutoff"), nil
	}
	adj, err := r.api.query(ctx, "adj_factor",
		map[string]any{"ts_code": symbol, "start_date": from, "end_date": asOf},
		"ts_code", "trade_date", "adj_factor")
	if err != nil {
		return nil, err
	}

	factors := make(map[string]float64, len(adj))
	for _, item := range adj {
		if str(item["ts_code"]) != symbol {
			return nil, errors.New("pool PIT factor returned a different symbol")
		}
		date := str(item["trade_date"])
		if !withinPoolWindow(date, from, asOf) {
			return nil, errors.New("pool PIT factor is outside the requested date window")
		}
		if _, exists := factors[date]; exists {
			return nil, errors.New("pool PIT factors contain duplicate trading date")
		}
		value := num(item["adj_factor"])
		// An invalid factor does not become an identity factor or a trade.
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("invalid pool PIT factor")
		}
		factors[date] = value
	}

	bars := make([]bar, 0, len(daily))
	for _, item := range daily {
		if str(item["ts_code"]) != symbol {
			return nil, errors.New("pool daily returned a different symbol")
		}
		date := str(item["trade_date"])
		if !withinPoolWindow(date, from, asOf) {
			return nil, errors.New("pool daily is outside the requested date window")
		}
		bars = append(bars, bar{
			Date: date,
			Open: num(item["open"]), High: num(item["high"]),
			Low: num(item["low"]), Close: num(item["close"]),
			Vol: num(item["vol"]) * 100, Amount: num(item["amount"]) * 1000,
		})
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date < bars[j].Date })
	history, err := observedHistoricalBars(bars)
	if err != nil {
		return nil, fmt.Errorf("invalid pool daily OHLCV: %w", err)
	}
	if len(history) == 0 {
		return statusMap(symbol, asOf, "NO_DATA", "", "empty pool daily observations"), nil
	}
	effective := history[len(history)-1].TradeDate
	rawHistory := map[string]any{
		"version": 1,
		"price_basis": "RAW",
		"bars": history,
	}
	allFactors := make([]PITAdjustmentFactor, 0, len(history))
	for _, candle := range history {
		factor, ok := factors[candle.TradeDate]
		if !ok {
			// Omit the entire factor series. The established
			// EvaluateObservation path will produce INSUFFICIENT_DATA,
			// MISSING_PIT_ADJUSTMENT; never synthesize factor=1.
			allFactors = nil
			break
		}
		allFactors = append(allFactors, PITAdjustmentFactor{
			TradeDate: candle.TradeDate, Factor: factor,
		})
	}
	if len(allFactors) == len(history) {
		rawHistory["pit_adjustment_factors"] = allFactors
	}
	return map[string]any{
		"status": "NORMAL",
		"symbol": symbol,
		"requested_as_of": asOf,
		"effective_as_of": effective,
		"historical_ohlcv": rawHistory,
	}, nil
}

func withinPoolWindow(date, start, end string) bool {
	if len(date) != 8 || date < start || date > end || strings.Trim(date, "0123456789") != "" {
		return false
	}
	d, err := time.Parse("20060102", date)
	return err == nil && d.Format("20060102") == date
}
