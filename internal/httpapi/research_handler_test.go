package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"Aegis/internal/application"
	"Aegis/internal/portfolio"
)

const testToken = "unit-test-long-secret-not-for-browser"

type fakeRuns struct {
	mu          sync.Mutex
	rec         application.RunRecord
	submitCalls int
	lastRequest application.ResearchRequest
	cancelCalls int
}

func (f *fakeRuns) Submit(_ context.Context, req application.ResearchRequest) (application.RunRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitCalls++
	f.lastRequest = req
	return f.rec, nil
}
func (f *fakeRuns) Get(_ context.Context, id string) (application.RunRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rec.ID != id {
		return application.RunRecord{}, application.ErrRunNotFound
	}
	return f.rec, nil
}
func (f *fakeRuns) List(_ context.Context) ([]application.RunRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []application.RunRecord{f.rec}, nil
}
func (f *fakeRuns) Cancel(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rec.ID != id {
		return application.ErrRunNotActive
	}
	f.cancelCalls++
	return nil
}
func mkHandler(t *testing.T, f *fakeRuns, mode application.ResearchMode) *Handler {
	t.Helper()
	h, e := NewHandler(f, testToken, mode, 7*24*time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	h.poll = time.Millisecond
	h.now = func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }
	return h
}
func apiRequest(t *testing.T, h http.Handler, method, path, body string, auth bool, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth {
		r.Header.Set("Authorization", "Bearer "+testToken)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func testRecord() application.RunRecord {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	return application.RunRecord{ID: strings.Repeat("a", 32), Mode: application.ModeDecision, Symbol: "600519.SH", State: application.RunCompleted, RequestedAsOf: now, CreatedAt: now, FinishedAt: &now, Artifact: json.RawMessage(`{"symbol":"600519.SH","action":"WATCH"}`), Events: []application.RunEvent{{Seq: 1, Stage: "Run", Status: "PENDING", At: now}, {Seq: 2, Stage: "Run", Status: "COMPLETED", At: now}}}
}
func TestHTTPAuthStrictInputAndPI(T *testing.T) {
	f := &fakeRuns{rec: testRecord()}
	h := mkHandler(T, f, application.ModeDecision)
	tests := []struct {
		body     string
		expected int
	}{
		{`{"symbol":"600519.SH"}`, 202},
		{`{"symbol":"bad"}`, 400},
		{`{"symbol":"600519.SH","unknown":1}`, 400},
		{`{"symbol":"600519.SH"} {}`, 400},
		{`{"symbol":"600519.SH","portfolio":{"state":"NOT_PROVIDED"}}`, 400},
		{`{"symbol":"600519.SH","mode":"PORTFOLIO_REVIEW"}`, 400},
	}
	for _, tc := range tests {
		r := apiRequest(T, h, "POST", "/api/v1/research/runs", tc.body, true, nil)
		if r.Code != tc.expected {
			T.Fatalf("payload %q got %d: %s", tc.body, r.Code, r.Body.String())
		}
	}
	if f.submitCalls != 1 || f.lastRequest.Input.Symbol != "600519.SH" || f.lastRequest.Input.RequestedAsOf.IsZero() {
		T.Fatalf("bad accepted input: %+v", f.lastRequest)
	}
	unauth := apiRequest(T, h, "GET", "/api/v1/research/runs", "", false, nil)
	if unauth.Code != 401 {
		T.Fatal("anonymous access allowed")
	}
	if strings.Contains(apiRequest(T, h, "GET", "/api/v1/research/runs", "", true, nil).Body.String(), "action") {
		T.Fatal("artifact exposed in list")
	}
}
func TestHTTPPortfolioUnknownMustBeExplicit(t *testing.T) {
	f := &fakeRuns{rec: testRecord()}
	h := mkHandler(t, f, application.ModePortfolio)
	fail := apiRequest(t, h, "POST", "/api/v1/research/runs", `{"symbol":"600519.SH"}`, true, nil)
	if fail.Code != 400 {
		t.Fatal("missing account accepted")
	}
	good := apiRequest(t, h, "POST", "/api/v1/research/runs", `{"symbol":"600519.SH","portfolio":{"state":"NOT_PROVIDED"}}`, true, nil)
	if good.Code != 202 || f.lastRequest.Portfolio == nil || f.lastRequest.Portfolio.State != portfolio.StateNotProvided {
		t.Fatalf("explicit unknown ignored: %d %s", good.Code, good.Body.String())
	}
}
func TestHTTPEndpointsAndReplay(t *testing.T) {
	f := &fakeRuns{rec: testRecord()}
	h := mkHandler(t, f, application.ModeDecision)
	id := f.rec.ID
	for path, want := range map[string]int{"/api/v1/research/runs/" + id: 200, "/api/v1/research/runs/" + id + "/artifact": 200, "/api/v1/research/runs/" + id + "/events": 200, "/api/v1/research/runs/" + id + "/cancel": 202} {
		method := "GET"
		if strings.HasSuffix(path, "/cancel") {
			method = "POST"
		}
		r := apiRequest(t, h, method, path, "", true, nil)
		if r.Code != want {
			t.Fatalf("%s got %d: %s", path, r.Code, r.Body.String())
		}
	}
	stream := apiRequest(t, h, "GET", "/api/v1/research/runs/"+id+"/stream", "", true, map[string]string{"Last-Event-ID": "1"})
	if stream.Code != 200 || strings.Contains(stream.Body.String(), "id: 1\n") || !strings.Contains(stream.Body.String(), "id: 2\n") || !strings.Contains(stream.Body.String(), "COMPLETED") {
		t.Fatalf("SSE resume incorrect: %d %s", stream.Code, stream.Body.String())
	}
	invalid := apiRequest(t, h, "GET", "/api/v1/research/runs/"+id+"/stream", "", true, map[string]string{"Last-Event-ID": "invalid"})
	if invalid.Code != 400 {
		t.Fatalf("accepted bad cursor %d", invalid.Code)
	}
	f.mu.Lock()
	f.rec.State = application.RunFailed
	f.rec.Artifact = nil
	f.mu.Unlock()
	noArtifact := apiRequest(t, h, "GET", "/api/v1/research/runs/"+id+"/artifact", "", true, nil)
	if noArtifact.Code != 409 {
		t.Fatalf("leaked invalid artifact: %d", noArtifact.Code)
	}
}

func TestLocalDevHandlerAllowsLoopbackDebugRequests(t *testing.T) {
	f := &fakeRuns{rec: testRecord()}
	h, err := NewLocalDevHandler(f, application.ModeDecision, 7*24*time.Hour)
	if err != nil { t.Fatal(err) }
	if w := apiRequest(t, h, http.MethodGet, "/api/v1/research/runs", "", false, nil); w.Code != http.StatusOK {
		t.Fatalf("local dev should allow unauthenticated requests: %d", w.Code)
	}
	production := mkHandler(t, f, application.ModeDecision)
	if w := apiRequest(t, production, http.MethodGet, "/api/v1/research/runs", "", false, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("production handler must still require bearer: %d", w.Code)
	}
}
