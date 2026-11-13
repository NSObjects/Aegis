package technical

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"
)

func poolBusinessDays(asOf string, count int) []string {
	end, _ := time.Parse("20060102", asOf)
	dates := make([]string, 0, count)
	for d := end; len(dates) < count; d = d.AddDate(0, 0, -1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		dates = append(dates, d.Format("20060102"))
	}
	return dates
}

func TestVerifiedPoolResearchReducesProviderRequestsAndFailsClosed(t *testing.T) {
	const asOf = "20261022"
	const symbol = "600519.SH"
	for _, tc := range []struct {
		name string
		missingFactor bool
		missingClose bool
		badSymbol bool
		duplicateBar bool
		wantError bool
		wantFactors int
		wantEffective string
	}{
		{name: "complete observed PIT", wantFactors: 70, wantEffective: asOf},
		{name: "missing historical factor is never replaced", missingFactor: true, wantEffective: asOf},
		{name: "missing exact cutoff remains stale", missingClose: true, wantFactors: 69, wantEffective: "20261021"},
		{name: "other symbol cannot contaminate pool", badSymbol: true, wantError: true},
		{name: "duplicate historical bar rejected", duplicateBar: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dates := poolBusinessDays(asOf, 70)
			// Defensive test assumption: the earlier weekday is Thursday.
			if dates[1] != "20261021" { t.Fatalf("bad test fixture: %q", dates[1]) }
			var mu sync.Mutex
			var calls []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				var req tushareRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("invalid provider request: %v", err)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				mu.Lock()
				calls = append(calls, req.APIName)
				mu.Unlock()
				if req.Params["ts_code"] != symbol ||
					req.Params["end_date"] != asOf || req.Params["start_date"] == nil ||
					req.Params["trade_date"] != nil {
					t.Errorf("unexpected parameters for %s: %+v", req.APIName, req.Params)
				}
				var fields []string
				var rows [][]any
				switch req.APIName {
				case "daily":
					fields = []string{"ts_code","trade_date","open","high","low","close","pre_close","pct_chg","vol","amount"}
					for index, day := range dates {
						if tc.missingClose && index == 0 { continue }
						code := symbol
						if tc.badSymbol && index == 1 { code = "000001.SZ" }
						rows = append(rows, []any{code,day,10.0,11.0,9.0,10.5,10.0,0.5,1000.0,20000.0})
					}
					if tc.duplicateBar { rows = append(rows, append([]any(nil), rows[0]...)) }
				case "adj_factor":
					fields = []string{"ts_code","trade_date","adj_factor"}
					for index, day := range dates {
						if tc.missingFactor && index == 35 { continue }
						if tc.missingClose && index == 0 { continue }
						rows = append(rows, []any{symbol,day,1.0 + float64(index)*0.001})
					}
				default:
					t.Errorf("optimized pool unexpectedly queried %s", req.APIName)
					http.Error(w, "unknown provider API", http.StatusBadRequest)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": 0,
					"data": map[string]any{"fields": fields, "items": rows},
				})
			}))
			defer srv.Close()
			client, err := NewTushareClient("fake-only")
			if err != nil { t.Fatal(err) }
			client.endpoint = srv.URL
			researcher := &HTTPResearcher{api: client}
			ctx := WithVerifiedPoolCutoff(context.Background(), asOf)
			result, found, err := ObserveHistorical(ctx, researcher, Request{Symbol:symbol, AsOf:asOf})
			if tc.wantError {
				if err == nil { t.Fatal("contaminated/duplicate pool observation accepted") }
			} else {
				if err != nil { t.Fatal(err) }
				if !found || result.EffectiveAsOf != tc.wantEffective || len(result.PITFactors) != tc.wantFactors {
					t.Fatalf("bad PIT observation found=%v result=%+v",found,result)
				}
				if len(result.Bars) < 65 { t.Fatal("lost valid OHLCV bars") }
			}
			mu.Lock()
			got := append([]string(nil), calls...)
			mu.Unlock()
			sort.Strings(got)
			if fmt.Sprint(got) != "[adj_factor daily]" {
				t.Fatalf("verified pool should use exactly two history requests, got %v",got)
			}
		})
	}
}

func TestVerifiedPoolCutoffDoesNotAffectGenericResearch(t *testing.T) {
	const asOf = "20261022"
	var names []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
		var req tushareRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		names = append(names, req.APIName)
		// Ensure the unverified path attempts the listing check before it
		// can reach any optimized history retrieval.
		_ = json.NewEncoder(w).Encode(map[string]any{"code": -1, "msg": "expected generic listing check"})
	}))
	defer server.Close()
	client, _ := NewTushareClient("fake-only")
	client.endpoint = server.URL
	r := &HTTPResearcher{api:client}
	_,err := r.Research(WithVerifiedPoolCutoff(context.Background(),"20261021"),
		Request{Symbol:"600519.SH",AsOf:asOf})
	if err == nil { t.Fatal("generic listing check should have failed") }
	if len(names) != 1 || names[0] != "stock_basic" {
		t.Fatalf("unverified stock bypassed listing check: %+v",names)
	}
}
