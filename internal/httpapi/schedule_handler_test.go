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

func newScheduleHandler(t *testing.T) http.Handler {
	t.Helper()
	store, err := application.NewFileScheduleStore(filepath.Join(t.TempDir(), "schedules"))
	if err != nil {
		t.Fatal(err)
	}
	return &Handler{localDev: true, schedules: store}
}

func TestScheduleHTTPCRUDAndValidation(t *testing.T) {
	handler := newScheduleHandler(t)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"tasks":[]`) {
		t.Fatalf("empty list %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"name":"每日回踩","kind":"SCAN","universe_source":"USER_SUPPLIED","symbols":["600519.SH"]}`)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created application.ScheduleTask
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.Kind != application.ScheduleKindScan || !created.Enabled {
		t.Fatalf("created: %+v %v", created, err)
	}
	id := created.ID
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/schedules/"+id, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("get %d", w.Code)
	}
	// Runs listing starts empty.
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/schedules/"+id+"/runs", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"runs":[]`) {
		t.Fatalf("runs %d %s", w.Code, w.Body.String())
	}
	// Delete refuses while enabled.
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/schedules/"+id, nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("delete enabled %d", w.Code)
	}
	// Validation failures never persist.
	for _, body := range []string{
		`{"name":"","kind":"SCAN","universe_source":"U","symbols":["600519.SH"]}`,
		`{"name":"x","kind":"BREW","symbol":"600519.SH"}`,
		`{"name":"x","kind":"SCAN","universe_source":"U","symbols":["600519.SH"],"extra":1}`,
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("invalid body accepted %d: %s", rec.Code, body)
		}
	}
	tasks, _ := handler.(*Handler).schedules.ListTasks()
	if len(tasks) != 1 {
		t.Fatalf("invalid bodies persisted: %d", len(tasks))
	}
	// Disable, then delete.
	disabled := created
	disabled.Enabled = false
	disabled.UpdatedAt = time.Now().UTC()
	if err := handler.(*Handler).schedules.UpdateTask(disabled); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/schedules/"+id, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete %d", w.Code)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/schedules/"+id, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("get deleted %d", w.Code)
	}
	// Auth and unwired service behavior.
	authed := &Handler{token: strings.Repeat("x", 32), schedules: handler.(*Handler).schedules}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	rec := httptest.NewRecorder()
	authed.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth %d", rec.Code)
	}
	unwired := &Handler{localDev: true}
	rec = httptest.NewRecorder()
	unwired.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired %d", rec.Code)
	}
}
