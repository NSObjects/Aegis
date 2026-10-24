package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"Aegis/internal/application"
	"Aegis/internal/capability/technical"
)

type fakeScans struct {
	mu        sync.Mutex
	rec       application.ScanRecord
	list      []application.ScanRecord
	submitted int
	lastReq   application.ScanRequest
	cancelErr error
	snapshot  []technical.HistoricalBar
	snapErr   error
}

func (f *fakeScans) Submit(_ context.Context, req application.ScanRequest) (application.ScanRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitted++
	f.lastReq = req
	return f.rec, nil
}
func (f *fakeScans) Get(_ context.Context, id string) (application.ScanRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rec.ID != id {
		return application.ScanRecord{}, application.ErrScanNotFound
	}
	return f.rec, nil
}
func (f *fakeScans) List(_ context.Context) ([]application.ScanRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.list != nil {
		return append([]application.ScanRecord(nil), f.list...), nil
	}
	return []application.ScanRecord{f.rec}, nil
}
func (f *fakeScans) Cancel(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cancelErr != nil {
		return f.cancelErr
	}
	if f.rec.ID != id {
		return application.ErrScanNotActive
	}
	return nil
}
func (f *fakeScans) GetSnapshot(_ context.Context, _, _ string) ([]technical.HistoricalBar, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshot, f.snapErr
}

func scanTestRecord() application.ScanRecord {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	return application.ScanRecord{
		ID: strings_32a(), State: application.ScanCompleted,
		RequestSnapshot: &application.ScanRequestSnapshot{SchemaVersion: 1, AsOf: "20261020", StrategyVersion: "trend-pullback-v1", UniversePolicy: "LISTED_SH_SZ", LookbackTradingDays: 90},
		Progress:        application.ScanProgress{SymbolsTotal: 2, SymbolsEvaluated: 2, DataDaysTotal: 90, DataDaysFetched: 90},
		Results: []application.ScanResultItem{
			{Symbol: "600519.SH", Verdict: "SELECTED", Reason: "TREND_PULLBACK_V1", BarsCount: 90, SnapshotSHA256: strings_64a()},
			{Symbol: "000001.SZ", Verdict: "REJECTED", Reason: "TREND_NOT_UP", BarsCount: 90, SnapshotSHA256: strings_64a()},
		},
		Summary:   &application.ScanSummary{Total: 2, Selected: 1, Rejected: 1},
		CreatedAt: now, FinishedAt: &now,
	}
}

func strings_32a() string { return repeatChar('a', 32) }
func strings_64a() string { return repeatChar('a', 64) }
func repeatChar(c byte, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = c
	}
	return string(out)
}

func mkScanHandler(t *testing.T, f *fakeScans) *Handler {
	t.Helper()
	h, err := NewHandler(&fakeRuns{}, testToken, application.ModeDecision, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h.now = func() time.Time { return time.Date(2026, 10, 24, 4, 0, 0, 0, time.UTC) }
	if f != nil {
		h = h.WithScanController(f)
	}
	return h
}

func TestScanRoutesUnavailableWithoutController(t *testing.T) {
	h := mkScanHandler(t, nil)
	w := apiRequest(t, h, http.MethodGet, "/api/v1/scans", "", true, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d want 503", w.Code)
	}
}

func TestScanSubmitAcceptsAndDefaultsAsOf(t *testing.T) {
	f := &fakeScans{rec: scanTestRecord()}
	h := mkScanHandler(t, f)
	w := apiRequest(t, h, http.MethodPost, "/api/v1/scans", `{"symbols":["600519.SH"]}`, true, map[string]string{"Idempotency-Key": "scan-key-0001"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/api/v1/scans/"+f.rec.ID {
		t.Fatalf("location=%s", loc)
	}
	// 04:00 UTC on 2026-10-24 is 12:00 in Asia/Shanghai: same calendar day.
	if f.lastReq.AsOf != "20261024" {
		t.Fatalf("default as_of=%s", f.lastReq.AsOf)
	}
	var echoed application.ScanRecord
	if err := json.Unmarshal(w.Body.Bytes(), &echoed); err != nil {
		t.Fatal(err)
	}
	if echoed.IdempotencyDigest != "" || echoed.RequestFingerprint != "" {
		t.Fatal("internal idempotency metadata leaked")
	}
}

func TestScanSubmitRejectsBadBodies(t *testing.T) {
	f := &fakeScans{rec: scanTestRecord()}
	h := mkScanHandler(t, f)
	cases := []struct{ name, body string }{
		{"unknown field", `{"as_of":"20261020","limit":1}`},
		{"bad date", `{"as_of":"2026-10-20"}`},
		{"bad symbol", `{"as_of":"20261020","symbols":["600519.SHX"]}`},
		{"trailing", `{"as_of":"20261020"} {}`},
	}
	for _, tc := range cases {
		w := apiRequest(t, h, http.MethodPost, "/api/v1/scans", tc.body, true, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s code=%d body=%s", tc.name, w.Code, w.Body.String())
		}
	}
	if f.submitted != 0 {
		t.Fatal("invalid submissions reached the controller")
	}
}

func TestScanListFilteringAndEnvelope(t *testing.T) {
	f := &fakeScans{rec: scanTestRecord()}
	h := mkScanHandler(t, f)
	w := apiRequest(t, h, http.MethodGet, "/api/v1/scans?state=COMPLETED&limit=1", "", true, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	var envelope struct {
		Scans []application.ScanRecord `json:"scans"`
		Total int                      `json:"total"`
		Limit int                      `json:"limit"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Total != 1 || len(envelope.Scans) != 1 || envelope.Limit != 1 {
		t.Fatalf("envelope=%+v", envelope)
	}
	if len(envelope.Scans[0].Results) != 0 {
		t.Fatal("listing rows must not carry per-symbol results")
	}
	if w := apiRequest(t, h, http.MethodGet, "/api/v1/scans?state=PAUSED", "", true, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("bad state filter code=%d", w.Code)
	}
}

func TestScanGetResultAndSnapshot(t *testing.T) {
	f := &fakeScans{rec: scanTestRecord()}
	f.snapshot = []technical.HistoricalBar{{TradeDate: "20261020", Open: 140, High: 141, Low: 139, Close: 140, Volume: 1000, Amount: 140000}}
	h := mkScanHandler(t, f)
	w := apiRequest(t, h, http.MethodGet, "/api/v1/scans/"+f.rec.ID, "", true, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get code=%d", w.Code)
	}
	w = apiRequest(t, h, http.MethodGet, "/api/v1/scans/"+f.rec.ID+"/results/600519.SH", "", true, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("result code=%d", w.Code)
	}
	if w := apiRequest(t, h, http.MethodGet, "/api/v1/scans/"+f.rec.ID+"/results/999999.SH", "", true, nil); w.Code != http.StatusNotFound {
		t.Fatalf("missing symbol code=%d", w.Code)
	}
	w = apiRequest(t, h, http.MethodGet, "/api/v1/scans/"+f.rec.ID+"/results/600519.SH/snapshot", "", true, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot code=%d body=%s", w.Code, w.Body.String())
	}
	var snapshot struct {
		Bars []technical.HistoricalBar `json:"bars"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil || len(snapshot.Bars) != 1 {
		t.Fatalf("snapshot bars err=%v", err)
	}
}

func TestScanSnapshotConflictsWhileRunning(t *testing.T) {
	running := scanTestRecord()
	running.State = application.ScanRunning
	f := &fakeScans{rec: running}
	f.snapErr = application.ErrScanNotTerminal
	h := mkScanHandler(t, f)
	w := apiRequest(t, h, http.MethodGet, "/api/v1/scans/"+running.ID+"/results/600519.SH/snapshot", "", true, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestScanCancelAndNotFound(t *testing.T) {
	f := &fakeScans{rec: scanTestRecord()}
	h := mkScanHandler(t, f)
	w := apiRequest(t, h, http.MethodPost, "/api/v1/scans/"+f.rec.ID+"/cancel", "", true, nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("cancel code=%d", w.Code)
	}
	if w := apiRequest(t, h, http.MethodPost, "/api/v1/scans/"+repeatChar('9', 32)+"/cancel", "", true, nil); w.Code != http.StatusConflict {
		t.Fatalf("inactive cancel code=%d", w.Code)
	}
	if w := apiRequest(t, h, http.MethodGet, "/api/v1/scans/"+repeatChar('9', 32), "", true, nil); w.Code != http.StatusNotFound {
		t.Fatalf("missing scan code=%d", w.Code)
	}
	if w := apiRequest(t, h, http.MethodGet, "/api/v1/scans/not-an-id", "", true, nil); w.Code != http.StatusNotFound {
		t.Fatalf("bad id code=%d", w.Code)
	}
}

func TestScanRoutesRequireAuth(t *testing.T) {
	f := &fakeScans{rec: scanTestRecord()}
	h := mkScanHandler(t, f)
	if w := apiRequest(t, h, http.MethodGet, "/api/v1/scans", "", false, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated code=%d", w.Code)
	}
}
