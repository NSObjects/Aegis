package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"Aegis/internal/application"
)

func newMonitorHandler(t *testing.T) http.Handler {
	t.Helper()
	store, err := application.NewFileMonitorStore(filepath.Join(t.TempDir(), "monitor"))
	if err != nil {
		t.Fatal(err)
	}
	return &Handler{localDev: true, monitors: store}
}

func TestMonitorHTTPCRUDAndEvents(t *testing.T) {
	handler := newMonitorHandler(t)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/monitors", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"rules":[]`) {
		t.Fatalf("empty list %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/monitors", strings.NewReader(`{"symbol":"600519.SH","kind":"PRICE_BELOW","threshold":1500,"cooldown_days":3}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created application.MonitorRule
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.Kind != application.MonitorPriceBelow || !created.Enabled {
		t.Fatalf("created: %+v %v", created, err)
	}
	// Validation refusals never persist.
	for _, body := range []string{
		`{"symbol":"600519","kind":"PRICE_BELOW","threshold":1}`,
		`{"symbol":"600519.SH","kind":"PRICE_BELOW","threshold":0}`,
		`{"symbol":"600519.SH","kind":"BREW"}`,
		`{"symbol":"600519.SH","kind":"PRICE_BELOW","threshold":1,"extra":1}`,
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/monitors", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("invalid body accepted %d: %s", rec.Code, body)
		}
	}
	// Record one outcome and list it.
	event := application.MonitorEvent{RuleID: created.ID, Symbol: created.Symbol, Kind: created.Kind, TradeDate: "20261023", Outcome: application.MonitorFired, Detail: "close=1499 threshold=1500", CreatedAt: created.UpdatedAt}
	if err := handler.(*Handler).monitors.(interface {
		SaveEvent(application.MonitorEvent) error
	}).SaveEvent(event); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/monitors/"+created.ID+"/events", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"outcome":"FIRED"`) {
		t.Fatalf("events %d %s", w.Code, w.Body.String())
	}
	// Events for an unknown rule report invalid, not a store failure.
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/monitors/ffffffffffffffff/events", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"events":[]`) {
		t.Fatalf("unknown rule events %d %s", w.Code, w.Body.String())
	}
	// Delete.
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/monitors/"+created.ID, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete %d", w.Code)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/monitors/"+created.ID, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("get deleted %d", w.Code)
	}
	// Unwired service reports unavailable like the other engines.
	unwired := &Handler{localDev: true}
	rec := httptest.NewRecorder()
	unwired.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/monitors", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired %d", rec.Code)
	}
}
