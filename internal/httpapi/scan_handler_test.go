package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"Aegis/internal/application"
	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

type fakeScans struct {
	last          application.ScanRecord
	snapshot      technical.HistoricalObservation
	snapErr       error
	comparison    application.ScanComparison
	comparisonErr error
	replay        application.ScanReplay
	replayErr     error
}

func (f *fakeScans) Submit(_ context.Context, asOf, source string, symbols []string) (application.ScanRecord, error) {
	f.last = application.ScanRecord{ID: "0123456789abcdef0123456789abcdef", AsOf: asOf, UniverseSource: source, Symbols: symbols, State: application.ScanPending, StrategyVersion: pullback.StrategyVersion, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	return f.last, nil
}
func (f *fakeScans) Get(id string) (application.ScanRecord, error) { return f.last, nil }
func (f *fakeScans) List(limit int) ([]application.ScanRecord, error) {
	return []application.ScanRecord{f.last}, nil
}
func (f *fakeScans) GetSnapshot(_, symbol string) (technical.HistoricalObservation, error) {
	if f.snapErr != nil {
		return technical.HistoricalObservation{}, f.snapErr
	}
	return f.snapshot, nil
}
func (f *fakeScans) Replay(string) (application.ScanReplay, error) {
	if f.replayErr != nil {
		return application.ScanReplay{}, f.replayErr
	}
	return f.replay, nil
}
func (f *fakeScans) Comparison(string) (application.ScanComparison, error) {
	if f.comparisonErr != nil {
		return application.ScanComparison{}, f.comparisonErr
	}
	return f.comparison, nil
}
func TestScanHTTPRoutesWithAuthAndInputValidation(t *testing.T) {
	fake := &fakeScans{}
	h := &Handler{token: strings.Repeat("x", 32), scans: fake}
	req := httptest.NewRequest("POST", "/api/v1/scans", strings.NewReader(`{"as_of":"20261022","universe_source":"USER_SUPPLIED","symbols":["600519.SH"]}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("without bearer: %d", w.Code)
	}
	req = httptest.NewRequest("POST", "/api/v1/scans", strings.NewReader(`{"as_of":"20261022","universe_source":"USER_SUPPLIED","symbols":["600519.SH"]}`))
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("submit %d %s", w.Code, w.Body.String())
	}
	var rec application.ScanRecord
	if err := json.Unmarshal(w.Body.Bytes(), &rec); err != nil || rec.ID == "" {
		t.Fatalf("invalid response %v", err)
	}
	req = httptest.NewRequest("GET", "/api/v1/scans/"+rec.ID, nil)
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get %d", w.Code)
	}
	req = httptest.NewRequest("POST", "/api/v1/scans", strings.NewReader(`{"as_of":"20261022","universe_source":"USER_SUPPLIED","symbols":["600519.SH"],"unexpected":true}`))
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown field accepted %d", w.Code)
	}
}

func TestScanHistoryQuery(t *testing.T) {
	h := &Handler{localDev: true, scans: &fakeScans{}}
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/v1/scans", http.StatusOK},
		{"/api/v1/scans?limit=1", http.StatusOK},
		{"/api/v1/scans?limit=0", http.StatusBadRequest},
		{"/api/v1/scans?limit=101", http.StatusBadRequest},
		{"/api/v1/scans?limit=bogus", http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s status=%d expected=%d", tc.path, w.Code, tc.want)
		}
	}
}

func TestScanSnapshotEndpoint(t *testing.T) {
	id := "0123456789abcdef0123456789abcdef"
	base := technical.HistoricalObservation{Symbol: "600519.SH", RequestedAsOf: "20261022", EffectiveAsOf: "20261022", PriceBasis: "RAW", Bars: []technical.HistoricalBar{{TradeDate: "20261022", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100, Amount: 1000}}}
	cases := []struct {
		name string
		fake *fakeScans
		want int
	}{
		{"serves observation", &fakeScans{snapshot: base}, http.StatusOK},
		{"missing symbol snapshot", &fakeScans{snapErr: application.ErrScanSnapshotNotFound}, http.StatusNotFound},
		{"unfinished scan", &fakeScans{snapErr: application.ErrScanNotCompleted}, http.StatusConflict},
		{"store miss", &fakeScans{snapErr: os.ErrNotExist}, http.StatusNotFound},
	}
	for _, tc := range cases {
		h := &Handler{localDev: true, scans: tc.fake}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/scans/"+id+"/results/600519.SH/snapshot", nil))
		if w.Code != tc.want {
			t.Errorf("%s status=%d expected=%d", tc.name, w.Code, tc.want)
		}
	}
	served := &fakeScans{snapshot: base}
	h := &Handler{localDev: true, scans: served}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/scans/"+id+"/results/600519.SH/snapshot", nil))
	var got technical.HistoricalObservation
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Symbol != "600519.SH" || len(got.Bars) != 1 {
		t.Fatalf("snapshot payload %+v %v", got, err)
	}
	if w := func() int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/scans/not-an-id/results/600519.SH/snapshot", nil))
		return rec.Code
	}(); w != http.StatusNotFound {
		t.Fatalf("bad scan id status=%d", w)
	}
}

func TestScanComparisonEndpoint(t *testing.T) {
	id := "0123456789abcdef0123456789abcdef"
	base := application.ScanComparison{ScanID: id, AsOf: "20261022", StrategyVersion: pullback.StrategyVersion, ComparisonVersion: pullback.ComparisonVersion, RankingRule: pullback.RankingRule,
		Ranked:       []application.ScanComparisonRanked{{Rank: 1, Symbol: "600519.SH", Source: application.ScanComparisonSource{SHA256: "a"}}},
		Incomparable: []application.ScanComparisonIncomparable{{Symbol: "000001.SZ", Verdict: pullback.Rejected, Reason: "TREND_NOT_UP", Note: "NOT_SELECTED"}},
	}
	cases := []struct {
		name string
		fake *fakeScans
		want int
	}{
		{"serves ranking", &fakeScans{comparison: base}, http.StatusOK},
		{"unfinished scan", &fakeScans{comparisonErr: application.ErrScanNotCompleted}, http.StatusConflict},
		{"missing scan", &fakeScans{comparisonErr: os.ErrNotExist}, http.StatusNotFound},
		{"digest failure", &fakeScans{comparisonErr: errors.New("snapshot digest mismatch")}, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		h := &Handler{localDev: true, scans: tc.fake}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/scans/"+id+"/comparison", nil))
		if w.Code != tc.want {
			t.Errorf("%s status=%d expected=%d", tc.name, w.Code, tc.want)
		}
	}
	h := &Handler{localDev: true, scans: &fakeScans{comparison: base}}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/scans/"+id+"/comparison", nil))
	var got application.ScanComparison
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.ScanID != id || len(got.Ranked) != 1 || len(got.Incomparable) != 1 {
		t.Fatalf("comparison payload %+v %v", got, err)
	}
	if got.Ranked[0].Rank != 1 || got.Incomparable[0].Note != "NOT_SELECTED" {
		t.Fatalf("rows wrong: %+v", got)
	}
	for _, path := range []string{"/api/v1/scans/" + id + "/comparison/extra", "/api/v1/scans/bad-id/comparison"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d", path, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/scans/"+id+"/comparison", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status=%d", rec.Code)
	}
}

func TestScanReplayEndpoint(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 base:=application.ScanReplay{ScanID:id,AsOf:"20261022",StrategyVersion:pullback.StrategyVersion,Replayable:true,Matches:true,
  Rows:[]application.ScanReplayRow{{Symbol:"600519.SH",Match:true,Source:application.ScanComparisonSource{SHA256:"a"}}},
  Unreplayable:[]application.ScanReplayUnreplayable{{Symbol:"000001.SZ",Reason:"NO_COMMITTED_SNAPSHOT"}},
 }
 cases:=[]struct{name string;fake *fakeScans;want int}{
  {"serves replay",&fakeScans{replay:base},http.StatusOK},
  {"unfinished scan",&fakeScans{replayErr:application.ErrScanNotCompleted},http.StatusConflict},
  {"missing scan",&fakeScans{replayErr:os.ErrNotExist},http.StatusNotFound},
  {"store failure",&fakeScans{replayErr:errors.New("snapshot digest mismatch")},http.StatusInternalServerError},
 }
 for _,tc:=range cases{
  h:=&Handler{localDev:true,scans:tc.fake}
  w:=httptest.NewRecorder()
  h.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/v1/scans/"+id+"/replay",nil))
  if w.Code!=tc.want{t.Errorf("%s status=%d expected=%d",tc.name,w.Code,tc.want)}
 }
 h:=&Handler{localDev:true,scans:&fakeScans{replay:base}}
 w:=httptest.NewRecorder()
 h.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/v1/scans/"+id+"/replay",nil))
 var got application.ScanReplay
 if err:=json.Unmarshal(w.Body.Bytes(),&got);err!=nil||got.ScanID!=id||!got.Matches||len(got.Rows)!=1{t.Fatalf("replay payload %+v %v",got,err)}
 rec:=httptest.NewRecorder()
 h.ServeHTTP(rec,httptest.NewRequest(http.MethodPost,"/api/v1/scans/"+id+"/replay",nil))
 if rec.Code!=http.StatusMethodNotAllowed{t.Fatalf("method status=%d",rec.Code)}
 rec=httptest.NewRecorder()
 h.ServeHTTP(rec,httptest.NewRequest(http.MethodGet,"/api/v1/scans/bad-id/replay",nil))
 if rec.Code!=http.StatusNotFound{t.Fatalf("bad id status=%d",rec.Code)}
}
