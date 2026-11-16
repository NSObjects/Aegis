package technical

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type marketProbeFixture struct {
	mu sync.Mutex
	calls []string
	universeSize int
	badFactor bool
}

func (f *marketProbeFixture) serve(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var q tushareRequest
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, "bad input", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.calls = append(f.calls, q.APIName)
	f.mu.Unlock()
	var fields []string
	rows := [][]any{}
	switch q.APIName {
	case "stock_basic":
		fields = []string{"ts_code", "list_date", "delist_date", "list_status"}
		if q.Params["list_status"] == "L" {
			for n := 0; n < f.universeSize; n++ {
				rows = append(rows, []any{fmt.Sprintf("600%03d.SH", n), "20000101", "", "L"})
			}
		}
	case "trade_cal":
		fields = []string{"cal_date", "is_open"}
		from, e1 := time.Parse("20060102", str(q.Params["start_date"]))
		to, e2 := time.Parse("20060102", str(q.Params["end_date"]))
		if e1 != nil || e2 != nil {
			http.Error(w, "invalid calendar range", http.StatusBadRequest)
			return
		}
		for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
			open := "1"
			if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
				open = "0"
			}
			rows = append(rows, []any{day.Format("20060102"), open})
		}
	case "daily":
		fields = []string{"ts_code", "trade_date", "open", "high", "low", "close", "vol", "amount"}
		date := str(q.Params["trade_date"])
		for n := 0; n < f.universeSize && n < 2; n++ {
			rows = append(rows, []any{fmt.Sprintf("600%03d.SH", n), date,
				10.0, 11.0, 9.0, 10.5, 1000.0, 10000.0})
		}
	case "adj_factor":
		fields = []string{"ts_code", "trade_date", "adj_factor"}
		date := str(q.Params["trade_date"])
		for n := 0; n < f.universeSize && n < 2; n++ {
			factor := 1.0
			if f.badFactor && n == 1 {
				factor = 0
			}
			rows = append(rows, []any{fmt.Sprintf("600%03d.SH", n), date, factor})
		}
	default:
		http.Error(w, "unsupported provider API", http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code": 0,
		"data": map[string]any{"fields": fields, "items": rows},
	})
}

func probeFixtureCache(t *testing.T, f *marketProbeFixture) (*MarketDayCache,func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	client, err := NewTushareClient("test-only-token")
	if err != nil { t.Fatal(err) }
	client.endpoint = srv.URL
	cache, err := NewMarketDayCache(client, filepath.Join(t.TempDir(), "probe"))
	if err != nil { t.Fatal(err) }
	cache.requestInterval = 0
	return cache,srv.Close
}

func (f *marketProbeFixture) count() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func TestMarketDayProbeBoundedRequestsAndCoverage(t *testing.T) {
	for _, days := range []int{1,3} {
		t.Run(fmt.Sprint(days), func(t *testing.T) {
			fixture := &marketProbeFixture{universeSize:2}
			cache, done := probeFixtureCache(t, fixture)
			defer done()
			report, err := cache.Probe(context.Background(), "20261022", days)
			if err != nil { t.Fatal(err) }
			if !report.ReadyForFullPool ||
				report.TradingDaysSampled != days ||
				report.MaxProviderCalls != 4+2*days ||
				len(report.Days) != days ||
				report.MainBoardSymbols != 2 {
				t.Fatalf("unexpected canary report: %+v", report)
			}
			for _, date := range report.Days {
				if date.DailyRows != 2 || date.FactorRows != 2 ||
					date.PairedRows != 2 || date.DailyBytes <= 0 || date.FactorBytes <= 0 {
					t.Fatalf("bad validated provider date: %+v", date)
				}
			}
			if got := len(fixture.count()); got != 4+2*days {
				t.Fatalf("canary exceeded provider request budget: %d", got)
			}
		})
	}
}

func TestMarketDayProbeRejectsSevereCoverageAndBadFactor(t *testing.T) {
	fixture := &marketProbeFixture{universeSize:100}
	cache, done := probeFixtureCache(t, fixture)
	defer done()
	report, err := cache.Probe(context.Background(), "20261022", 1)
	if err != nil { t.Fatal(err) }
	if report.ReadyForFullPool || !report.Days[0].SevereUndercover ||
		report.Days[0].MainBoardDaily != 2 || len(report.Warnings) == 0 {
		t.Fatalf("incomplete market treated as ready: %+v", report)
	}
	if got := len(fixture.count()); got != 6 {
		t.Fatalf("unexpected API count: %d", got)
	}

	bad := &marketProbeFixture{universeSize:2, badFactor:true}
	badCache, closeBad := probeFixtureCache(t, bad)
	defer closeBad()
	_, err = badCache.Probe(context.Background(), "20261022", 1)
	if err == nil || !strings.Contains(err.Error(), "adjustment factors") {
		t.Fatalf("invalid provider factor was accepted: %v", err)
	}
	if got := len(bad.count()); got != 6 {
		t.Fatalf("invalid factor should not trigger retries, got %d", got)
	}
}

func TestMarketDayProbeRejectsInvalidInputBeforeNetwork(t *testing.T) {
	fixture := &marketProbeFixture{universeSize:2}
	cache, done := probeFixtureCache(t, fixture)
	defer done()
	for _, tc := range []struct{ asOf string; days int }{
		{"20261022",0}, {"20261022",4}, {"20261399",1},
	} {
		if _,err := cache.Probe(context.Background(),tc.asOf,tc.days);err==nil {
			t.Fatalf("invalid probe accepted: %+v", tc)
		}
	}
	if len(fixture.count()) != 0 {t.Fatal("invalid parameters caused provider calls")}
}
