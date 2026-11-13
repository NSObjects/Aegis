package technical

import (
	"context"
	"fmt"
	"time"
)

// TradingCalendar answers one question for the scheduler: was the given
// YYYYMMDD date an SSE trading day? Providers own the calendar; callers never
// guess weekends or holidays locally.
type TradingCalendar interface {
	IsTradingDay(ctx context.Context, date string) (bool, error)
}

// TushareTradingCalendar reads SSE trade_cal through the existing client and
// caches the answer per date for the process lifetime: the calendar is
// published history, not a mutable source.
type TushareTradingCalendar struct {
	client *TushareClient
	cache  map[string]bool
}

func NewTushareTradingCalendar(client *TushareClient) (*TushareTradingCalendar, error) {
	if client == nil {
		return nil, fmt.Errorf("tushare client required")
	}
	return &TushareTradingCalendar{client: client, cache: map[string]bool{}}, nil
}

func validCalendarDate(date string) bool {
	if len(date) != 8 {
		return false
	}
	_, err := time.Parse("20060102", date)
	return err == nil
}

func (c *TushareTradingCalendar) IsTradingDay(ctx context.Context, date string) (bool, error) {
	if c == nil || c.client == nil {
		return false, fmt.Errorf("nil trading calendar")
	}
	if !validCalendarDate(date) {
		return false, fmt.Errorf("invalid calendar date %q", date)
	}
	if open, ok := c.cache[date]; ok {
		return open, nil
	}
	rows, err := c.client.query(ctx, "trade_cal", map[string]any{"exchange": "SSE", "start_date": date, "end_date": date}, "cal_date", "is_open")
	if err != nil {
		return false, fmt.Errorf("trade calendar query: %w", err)
	}
	if len(rows) != 1 {
		return false, fmt.Errorf("trade calendar returned %d rows for %s", len(rows), date)
	}
	raw, ok := rows[0]["is_open"]
	if !ok {
		return false, fmt.Errorf("trade calendar missing is_open for %s", date)
	}
	var open bool
	switch v := raw.(type) {
	case string:
		open = v == "1"
	case float64:
		open = v == 1
	default:
		return false, fmt.Errorf("unexpected is_open type %T for %q", raw, date)
	}
	if rows[0]["cal_date"] != date {
		return false, fmt.Errorf("trade calendar date mismatch %q", rows[0]["cal_date"])
	}
	c.cache[date] = open
	return open, nil
}
