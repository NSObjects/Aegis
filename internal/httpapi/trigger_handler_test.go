package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Aegis/internal/application"
)

func TestTriggerHTTPAuditTrail(t *testing.T) {
	store, err := application.NewFileTriggerStore(filepath.Join(t.TempDir(), "triggers"))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{localDev: true, triggers: store}
	now := time.Now().UTC()
	record := application.TriggerRecord{
		ID: "0123456789abcdef261023", RuleID: "0123456789abcdef", Symbol: "600519.SH",
		MonitorKind: application.MonitorAnnouncement, TradeDate: "20261023",
		Route: application.RouteThesisReview, Reason: "announcements=1", RunID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		State: application.ScheduleRunning, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/triggers", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"route":"THESIS_REVIEW"`) || !strings.Contains(w.Body.String(), `"run_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`) {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Triggers []application.TriggerRecord `json:"triggers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil || len(listed.Triggers) != 1 {
		t.Fatalf("parse: %v", err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/triggers/0123456789abcdef261023", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("get %d", w.Code)
	}
	// Missing-but-valid id is 404; malformed id is 400; limit is bounded.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/triggers/0123456789abcdef261099", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/triggers/not-an-id", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/triggers?limit=0", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("limit %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/triggers", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method %d", w.Code)
	}
	// Read-only: no write surface exists at all; unwired reports 503.
	unwired := &Handler{localDev: true}
	rec := httptest.NewRecorder()
	unwired.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/triggers", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired %d", rec.Code)
	}
}
