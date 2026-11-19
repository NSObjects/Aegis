package httpapi

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Aegis/internal/application"
	"Aegis/internal/settings"
)

func TestPortfolioAccountVersionsRequireAuthenticationAndPreserveAvailability(t *testing.T) {
	const path = "/api/v1/portfolio/account-versions"
	store, err := settings.NewPortfolioAccountStore(filepath.Join(t.TempDir(), "private", "versions.enc"), strings.Repeat("aa", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := mkHandler(t, &fakeRuns{}, application.ModeDecision).WithPortfolioAccounts(store)
	now := h.now().UTC()
	book := `{"snapshot":{"account_id":"demo","version":"v1","source":{"kind":"OPERATOR","reference":"synthetic"},"book":{"state":"EMPTY","as_of":"` +
		now.Add(-time.Minute).Format(time.RFC3339) + `","valuation":{"scope":"CASH_AND_POSITIONS","currency":"CNY","position_values":[],"total_value_fen":0,"cash_fen":0}}}}`
	for _, tc := range []struct {
		method, target, body string
		auth                 bool
		status               int
	}{
		{http.MethodPost, path, book, false, http.StatusUnauthorized},
		{http.MethodPost, path, book, true, http.StatusCreated},
		{http.MethodPost, path, book, true, http.StatusOK},
		{http.MethodGet, path + "?account_id=demo&snapshot_version=v1", "", false, http.StatusUnauthorized},
		{http.MethodGet, path + "?account_id=demo&snapshot_version=v1", "", true, http.StatusOK},
		{http.MethodGet, path + "?account_id=demo&snapshot_version=other", "", true, http.StatusNotFound},
		{http.MethodGet, path + "?account_id=demo&snapshot_version=v1&as_of=" + now.Add(-time.Second).Format(time.RFC3339), "", true, http.StatusUnprocessableEntity},
	} {
		w := apiRequest(t, h, tc.method, tc.target, tc.body, tc.auth, nil)
		if w.Code != tc.status {
			t.Fatalf("%s %s returned %d, want %d: %s", tc.method, tc.target, w.Code, tc.status, w.Body)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("sensitive account response is cacheable")
		}
	}
	dev, err := NewLocalDevHandler(&fakeRuns{}, application.ModeDecision, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	dev.WithPortfolioAccounts(store)
	if w := apiRequest(t, dev, http.MethodPost, path, book, false, nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("loopback dev must not expose account import: %d", w.Code)
	}
}
