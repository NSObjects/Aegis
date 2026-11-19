package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Aegis/internal/application"
)

func TestResearchHypothesisBoundToCompletedArtifactAndRetried(t *testing.T) {
	const id = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	asOf := time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)
	runs := &fakeRuns{rec: application.RunRecord{
		ID: id, Symbol: "600519.SH", State: application.RunCompleted,
		RequestedAsOf: asOf, Artifact: json.RawMessage(`{"decision":"HOLD"}`),
	}}
	h := mkHandler(t, runs, application.ModeDecision)
	dir := filepath.Join(t.TempDir(), "hypotheses")
	reopen := func() {
		store, err := application.NewFileHypothesisStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		h.WithHypothesisStore(store)
	}
	reopen()
	path := "/api/v1/research/runs/" + id + "/hypotheses"
	const draft = `{"statement":"回购或改善流动性","expires_at":"2026-12-01T18:00:00+08:00","supporting_evidence":[{"reference":"公告 A","detail":"披露拟回购金额"}],"contrary_evidence":[{"reference":"报告风险段","detail":"成交额下行"}],"invalidation":"公告被撤回","verification_rule":"在有效期内比较成交额与研究日基线"}`

	check := func(method, target, body string, auth bool, expected int) []byte {
		t.Helper()
		response := apiRequest(t, h, method, target, body, auth, nil)
		if response.Code != expected {
			t.Fatalf("%s %s: got %d, want %d: %s", method, target, response.Code, expected, response.Body)
		}
		return response.Body.Bytes()
	}

	check(http.MethodPost, path, draft, false, http.StatusUnauthorized)
	check(http.MethodPost, strings.Replace(path, id, strings.Repeat("b", 32), 1), draft, true, http.StatusNotFound)
	runs.rec.State = application.RunRunning
	check(http.MethodPost, path, draft, true, http.StatusConflict)
	runs.rec.State = application.RunCompleted
	check(http.MethodPost, path, strings.Replace(draft, "2026-12-01", "2026-10-01", 1), true, http.StatusBadRequest)
	check(http.MethodPost, path, `{"run_id":"other",`+draft[1:], true, http.StatusBadRequest)

	created := check(http.MethodPost, path, draft, true, http.StatusCreated)
	reopen()
	h.now = time.Now
	retry := check(http.MethodPost, path, draft, true, http.StatusOK)
	if string(created) != string(retry) {
		t.Fatalf("retry changed persisted annotation: %s / %s", created, retry)
	}

	var response struct {
		Hypotheses []application.ResearchHypothesis `json:"hypotheses"`
	}
	if err := json.Unmarshal(check(http.MethodGet, path, "", true, http.StatusOK), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Hypotheses) != 1 {
		t.Fatalf("want one persisted hypothesis, got %d", len(response.Hypotheses))
	}
	record := response.Hypotheses[0]
	if record.RunID != id || record.Symbol != runs.rec.Symbol || !record.AsOf.Equal(asOf) ||
		len(record.ArtifactSHA256) != 64 || len(record.Supporting) != 1 || len(record.Contrary) != 1 {
		t.Fatalf("unexpected persisted hypothesis: %+v", record)
	}
	if actual := check(http.MethodGet, "/api/v1/research/runs/"+id+"/artifact", "", true, http.StatusOK); string(actual) != string(runs.rec.Artifact) {
		t.Fatalf("source report changed: %s", actual)
	}
	observations := path + "/" + record.ID + "/observations"
	const followUpID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	runs.rec.ID = followUpID
	runs.rec.RequestedAsOf = asOf.Add(48 * time.Hour)
	const followUp = `{"run_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`
	check(http.MethodPost, observations, followUp, false, http.StatusUnauthorized)
	posted := check(http.MethodPost, observations, followUp, true, http.StatusCreated)
	var observed application.HypothesisObservation
	if err := json.Unmarshal(posted, &observed); err != nil || observed.RunID != followUpID || observed.Status != "INCONCLUSIVE" {
		t.Fatalf("legacy text rule yielded an invalid observation: %+v, %v", observed, err)
	}
	reopen()
	check(http.MethodPost, observations, followUp, true, http.StatusOK)
	var loaded struct {
		Observations []application.HypothesisObservation `json:"observations"`
	}
	if err := json.Unmarshal(check(http.MethodGet, observations, "", true, http.StatusOK), &loaded); err != nil || len(loaded.Observations) != 1 {
		t.Fatalf("unstable observation history: %+v, %v", loaded, err)
	}

	reviews := path + "/" + record.ID + "/reviews"
	const review = `{"observation_run_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","category":"DATA_MISSING","reason":"缺少目标日成交量","supporting_evidence":[{"reference":"后续报告行情段","detail":"没有成交量记录"}],"counterexamples":[{"reference":"公司公告","detail":"公告仍可查证"}]}`
	check(http.MethodPost, reviews, review, false, http.StatusUnauthorized)
	check(http.MethodPost, reviews, strings.Replace(review, "DATA_MISSING", "UNKNOWN", 1), true, http.StatusBadRequest)
	check(http.MethodPost, reviews, strings.Replace(review, followUpID, id, 1), true, http.StatusConflict)
	saved := check(http.MethodPost, reviews, review, true, http.StatusCreated)
	reopen()
	if retried := check(http.MethodPost, reviews, review, true, http.StatusOK); string(retried) != string(saved) {
		t.Fatalf("retry changed the review: %s / %s", saved, retried)
	}
	var archive struct {
		Reviews []application.HypothesisReview `json:"reviews"`
	}
	if err := json.Unmarshal(check(http.MethodGet, reviews, "", true, http.StatusOK), &archive); err != nil || len(archive.Reviews) != 1 {
		t.Fatalf("unstable review archive: %+v, %v", archive, err)
	}
	got := archive.Reviews[0]
	if got.HypothesisID != record.ID || got.SourceRunID != id || got.ObservationRunID != observed.RunID ||
		got.Category != "DATA_MISSING" || len(got.Supporting) != 1 || len(got.Counterexamples) != 1 {
		t.Fatalf("review lost evidence or provenance: %+v", got)
	}

	const memory = "/api/v1/research/memory"
	source := application.MemorySource{
		SourceRunID: id, HypothesisID: record.ID, ObservationRunID: followUpID, ReviewID: got.ID, MarketPhase: "盘整",
	}
	body, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	check(http.MethodPost, memory, string(body), false, http.StatusUnauthorized)
	check(http.MethodPost, memory, strings.Replace(string(body), got.ID, strings.Repeat("f", 32), 1), true, http.StatusConflict)
	first := check(http.MethodPost, memory, string(body), true, http.StatusCreated)
	reopen()
	if retry := check(http.MethodPost, memory, string(body), true, http.StatusOK); string(retry) != string(first) {
		t.Fatalf("duplicate memory differs: %s", retry)
	}
	var entry application.ResearchMemory
	if err := json.Unmarshal(first, &entry); err != nil || entry.Category != "DATA_MISSING" ||
		entry.Outcome != "INCONCLUSIVE" || entry.Thesis != record.Statement || entry.ObservationRunID != followUpID {
		t.Fatalf("wrong memory provenance: %+v, %v", entry, err)
	}
	checkMemory := func(query string, total int) {
		t.Helper()
		var results application.MemoryResults
		if err := json.Unmarshal(check(http.MethodGet, memory+query, "", true, http.StatusOK), &results); err != nil ||
			results.Total != total || len(results.Memories) != total {
			t.Fatalf("search count mismatch: %+v, %v", results, err)
		}
	}
	checkMemory("?symbol=600519.sh&q=%E5%9B%9E%E8%B4%AD&market_phase=%E7%9B%98%E6%95%B4&category=DATA_MISSING&outcome=INCONCLUSIVE", 1)
	checkMemory("?category=CITATION_ERROR", 0)
	checkMemory("?outcome=SUPPORTED", 0)
	revokePath := memory + "/" + entry.ID + "/revoke"
	check(http.MethodPost, revokePath, "", true, http.StatusOK)
	reopen()
	check(http.MethodPost, revokePath, "", true, http.StatusOK)
	check(http.MethodPost, memory, string(body), true, http.StatusConflict)
	checkMemory("", 0)
	check(http.MethodGet, reviews, "", true, http.StatusOK)

}
