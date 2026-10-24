package technical

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newFakeTushare serves Tushare-shaped responses and records API calls.
func newFakeTushare(t *testing.T, respond func(api string, params map[string]any) any) *TushareBatchSource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			APIName string         `json:"api_name"`
			Params  map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		payload, err := json.Marshal(respond(req.APIName, req.Params))
		if err != nil {
			t.Errorf("marshal response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	// Same-package tests may construct the private client directly.
	return &TushareBatchSource{api: &TushareClient{token: "test", endpoint: server.URL, client: server.Client()}}
}

func calendarResponse(days [][2]string) any {
	fields := []string{"cal_date", "is_open"}
	items := make([][]any, 0, len(days))
	for _, day := range days {
		items = append(items, []any{day[0], day[1]})
	}
	return map[string]any{"code": 0, "data": map[string]any{"fields": fields, "items": items}}
}

func TestBatchSourceTradingDaysFiltersAndTruncates(t *testing.T) {
	source := newFakeTushare(t, func(api string, params map[string]any) any {
		if api != "trade_cal" {
			t.Errorf("api=%s", api)
		}
		if params["is_open"] != "1" {
			t.Errorf("calendar must request open days only: %v", params)
		}
		if params["end_date"] != "20260112" {
			t.Errorf("end_date=%v", params["end_date"])
		}
		// Unsorted mixed open/closed days; the client must filter locally
		// rather than trusting the provider's is_open filtering.
		return calendarResponse([][2]string{
			{"20260101", "0"}, {"20260112", "1"}, {"20260105", "1"},
			{"20260108", "1"}, {"20260109", "0"}, {"20260106", "1"}, {"20260107", "1"},
		})
	})
	days, err := source.TradingDays(context.Background(), "20260112", 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"20260107", "20260108", "20260112"}
	if len(days) != len(want) {
		t.Fatalf("days=%v", days)
	}
	for i := range want {
		if days[i] != want[i] {
			t.Fatalf("days=%v want %v", days, want)
		}
	}
}

func TestBatchSourceTradingDaysRejectsBadAsOfAndEmptyCalendar(t *testing.T) {
	source := newFakeTushare(t, func(string, map[string]any) any {
		return calendarResponse(nil)
	})
	if _, err := source.TradingDays(context.Background(), "2026-1-12", 3); err == nil {
		t.Fatal("malformed as-of accepted")
	}
	if _, err := source.TradingDays(context.Background(), "20260112", 3); err == nil {
		t.Fatal("empty calendar accepted")
	}
	if _, err := source.TradingDays(context.Background(), "20260112", 0); err == nil {
		t.Fatal("non-positive window accepted")
	}
}

func TestBatchSourceListedSymbolsValidatesCoverage(t *testing.T) {
	normal := newFakeTushare(t, func(string, map[string]any) any {
		return map[string]any{"code": 0, "data": map[string]any{"fields": []string{"ts_code"}, "items": [][]any{{"300001.SZ"}, {"600519.SH"}, {"830001.BJ"}, {"600519.SH"}}}}
	})
	symbols, err := normal.ListedSymbols(context.Background())
	if err != nil || len(symbols) != 4 || symbols[0] != "300001.SZ" {
		t.Fatalf("symbols=%v err=%v", symbols, err)
	}
	empty := newFakeTushare(t, func(string, map[string]any) any {
		return map[string]any{"code": 0, "data": map[string]any{"fields": []string{"ts_code"}, "items": [][]any{}}}
	})
	if _, err := empty.ListedSymbols(context.Background()); err == nil {
		t.Fatal("empty listing accepted")
	}
	truncated := newFakeTushare(t, func(string, map[string]any) any {
		items := make([][]any, 6000)
		for i := range items {
			items[i] = []any{"600000.SH"}
		}
		return map[string]any{"code": 0, "data": map[string]any{"fields": []string{"ts_code"}, "items": items}}
	})
	if _, err := truncated.ListedSymbols(context.Background()); err == nil {
		t.Fatal("page-limit listing accepted as complete")
	}
}

func TestBatchSourceDailyBarsConvertsUnitsAndDropsForeignRows(t *testing.T) {
	source := newFakeTushare(t, func(api string, params map[string]any) any {
		if api != "daily" || params["trade_date"] != "20260112" {
			t.Errorf("api=%s params=%v", api, params)
		}
		return map[string]any{"code": 0, "data": map[string]any{"fields": []string{"ts_code", "trade_date", "open", "high", "low", "close", "vol", "amount"}, "items": [][]any{
			{"600519.SH", "20260112", "100.0", "101.0", "99.0", "100.5", "1234.0", "5678.0"},
			{"000001.SZ", "20260112", 10.0, 11.0, 9.0, 10.0, 1.0, 2.0},
			{"600000.SH", "20260111", 10.0, 10.0, 10.0, 10.0, 1.0, 1.0}, // wrong date: skipped
		}}}
	})
	bars, err := source.DailyBars(context.Background(), "20260112")
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 {
		t.Fatalf("bars=%d", len(bars))
	}
	first := bars[0]
	if first.Symbol != "600519.SH" || first.TradeDate != "20260112" || first.Open != 100 || first.Close != 100.5 {
		t.Fatalf("first=%+v", first)
	}
	// Lots to shares and thousands of yuan to yuan.
	if first.Volume != 123400 || first.Amount != 5678000 {
		t.Fatalf("unit conversion: %+v", first)
	}
	if bars[1].Volume != 100 || bars[1].Amount != 2000 {
		t.Fatalf("numeric conversion: %+v", bars[1])
	}
	if _, err := source.DailyBars(context.Background(), "2026/01/12"); err == nil {
		t.Fatal("malformed trade date accepted")
	}
}

func TestBatchSourceDailyBarsFailsClosedOnInvalidOHLCV(t *testing.T) {
	source := newFakeTushare(t, func(string, map[string]any) any {
		return map[string]any{"code": 0, "data": map[string]any{"fields": []string{"ts_code", "trade_date", "open", "high", "low", "close", "vol", "amount"}, "items": [][]any{
			{"600519.SH", "20260112", "-1.0", "101.0", "99.0", "100.5", "1.0", "1.0"},
		}}}
	})
	if _, err := source.DailyBars(context.Background(), "20260112"); err == nil {
		t.Fatal("negative price accepted")
	}
}

func TestBatchSourcePropagatesProviderErrors(t *testing.T) {
	source := newFakeTushare(t, func(api string, _ map[string]any) any {
		return map[string]any{"code": 1001, "msg": "积分不足或无权限"}
	})
	if _, err := source.TradingDays(context.Background(), "20260112", 3); err == nil || !strings.Contains(err.Error(), "trade_cal") {
		t.Fatalf("calendar err=%v", err)
	}
	if _, err := source.ListedSymbols(context.Background()); err == nil {
		t.Fatal("listing error swallowed")
	}
	if _, err := source.DailyBars(context.Background(), "20260112"); err == nil {
		t.Fatal("daily error swallowed")
	}
}
