package technical

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func calendarFixture(date, isOpen string) string {
	return `{"code":0,"msg":"","data":{"fields":["cal_date","is_open"],"items":[["` + date + `","` + isOpen + `"]]}}`
}

func newCalendarTestClient(t *testing.T, handler http.HandlerFunc) (*TushareTradingCalendar, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewTushareClient("fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	client.endpoint = server.URL
	cal, err := NewTushareTradingCalendar(client)
	if err != nil {
		t.Fatal(err)
	}
	return cal, server
}

func TestTushareTradingCalendarParsesIsOpen(t *testing.T) {
	cal, _ := newCalendarTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := readAllBody(t, r)
		if !strings.Contains(body, `"api_name":"trade_cal"`) {
			t.Errorf("unexpected api: %s", body)
		}
		switch {
		case strings.Contains(body, `"start_date":"20261024"`):
			// Saturday
			_, _ = w.Write([]byte(calendarFixture("20261024", "0")))
		case strings.Contains(body, `"start_date":"20261023"`):
			// Numeric is_open must parse too.
			_, _ = w.Write([]byte(`{"code":0,"msg":"","data":{"fields":["cal_date","is_open"],"items":[["20261023",1]]}}`))
		case strings.Contains(body, `"start_date":"20261025"`):
			http.Error(w, "provider down", http.StatusBadGateway)
		case strings.Contains(body, `"start_date":"20261026"`):
			// A mismatched row must be refused.
			_, _ = w.Write([]byte(calendarFixture("20261027", "1")))
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	})
	ctx := context.Background()
	open, err := cal.IsTradingDay(ctx, "20261024")
	if err != nil || open {
		t.Fatalf("saturday open=%v err=%v", open, err)
	}
	open, err = cal.IsTradingDay(ctx, "20261023")
	if err != nil || !open {
		t.Fatalf("friday open=%v err=%v", open, err)
	}
	if _, err := cal.IsTradingDay(ctx, "20261025"); err == nil {
		t.Fatal("provider error swallowed")
	}
	if _, err := cal.IsTradingDay(ctx, "20261026"); err == nil {
		t.Fatal("date mismatch accepted")
	}
	if _, err := cal.IsTradingDay(ctx, "2026-10-24"); err == nil {
		t.Fatal("invalid date accepted")
	}
	if _, err := cal.IsTradingDay(ctx, "2026102"); err == nil {
		t.Fatal("short date accepted")
	}
}

func TestTushareTradingCalendarCachesPerDate(t *testing.T) {
	queries := 0
	cal, _ := newCalendarTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		queries++
		_, _ = w.Write([]byte(calendarFixture(extractStartDate(t, r), "1")))
	})
	for i := 0; i < 3; i++ {
		if open, err := cal.IsTradingDay(context.Background(), "20261023"); err != nil || !open {
			t.Fatalf("call %d: %v %v", i, open, err)
		}
	}
	if queries != 1 {
		t.Fatalf("calendar re-queried same date: %d", queries)
	}
	if open, err := cal.IsTradingDay(context.Background(), "20261022"); err != nil || !open {
		t.Fatalf("second date: %v %v", open, err)
	}
	if queries != 2 {
		t.Fatalf("expected 2 queries, got %d", queries)
	}
}

func extractStartDate(t *testing.T, r *http.Request) string {
	t.Helper()
	body := readAllBody(t, r)
	marker := `"start_date":"`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no start_date in %s", body)
	}
	rest := body[i+len(marker):]
	return rest[:strings.Index(rest, `"`)]
}

func readAllBody(t *testing.T, r *http.Request) string {
	t.Helper()
	raw := make([]byte, 4096)
	n, _ := r.Body.Read(raw)
	return string(raw[:n])
}
