package httpapi

import (
 "bufio"
 "context"
 "io"
 "net/http/httptest"
 "net/http"
 "strings"
 "testing"
 "time"

 "Aegis/internal/application"
)

// Tool events are emitted by real execution hooks. This tests their transport
// with a fixed protocol fixture, not a paid model or source provider.
func TestToolTraceEventsHTTPAndSSEReplay(t *testing.T) {
 record := testRecord()
 now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
 record.Events = []application.RunEvent{
  {Seq:1, Stage:"Run", Status:"PENDING", At:now},
  {Seq:2, Stage:"Event", Status:"START", At:now},
  {Seq:3, Kind:"TOOL", Stage:"Event", Tool:"snapshot_official", Status:"START", At:now},
  {Seq:4, Kind:"TOOL", Stage:"Event", Tool:"snapshot_official", Status:"FAIL", DurationMS:45, At:now},
  {Seq:5, Stage:"Event", Status:"DONE", DurationMS:70, At:now},
  {Seq:6, Stage:"Run", Status:"COMPLETED", At:now},
 }
 h := mkHandler(t, &fakeRuns{rec:record}, application.ModeDecision)
 path := "/api/v1/research/runs/" + record.ID
 unauth := apiRequest(t,h,"GET",path+"/events","",false,nil)
 if unauth.Code != http.StatusUnauthorized {
  t.Fatalf("unauthenticated events returned %d",unauth.Code)
 }
 events := apiRequest(t,h,"GET",path+"/events","",true,nil)
 if events.Code != http.StatusOK || !strings.Contains(events.Body.String(),`"tool":"snapshot_official"`) || !strings.Contains(events.Body.String(),`"status":"FAIL"`) {
  t.Fatalf("tool metadata missing from events: %d %s",events.Code,events.Body.String())
 }
 stream := apiRequest(t,h,"GET",path+"/stream","",true,map[string]string{"Last-Event-ID":"3"})
 if stream.Code != http.StatusOK || strings.Contains(stream.Body.String(),"id: 3\n") ||
  !strings.Contains(stream.Body.String(),"id: 4\n") ||
  !strings.Contains(stream.Body.String(),"id: 6\n") ||
  !strings.Contains(stream.Body.String(),`"tool":"snapshot_official"`) {
  t.Fatalf("SSE replay lost tool event or cursor handling: %d %s",stream.Code,stream.Body.String())
 }
}

// Regression: a replay cursor belongs to the requested Run; it must not
// expose events for a different Run even if the cursor value matches.
func TestSSEReplayDoesNotCrossRunBoundary(t *testing.T) {
 first := testRecord()
 second := testRecord()
 second.ID = strings.Repeat("b", 32)
 second.Symbol = "000001.SZ"
 second.Events = []application.RunEvent{
  {Seq:1, Stage:"Run", Status:"PENDING", At:first.CreatedAt},
  {Seq:2, Stage:"Event", Status:"FAIL", At:first.CreatedAt},
 }
 first.Events = []application.RunEvent{
  {Seq:1, Stage:"Run", Status:"PENDING", At:first.CreatedAt},
  {Seq:2, Stage:"Market", Status:"DONE", At:first.CreatedAt},
 }
 for _, tc := range []struct{ rec application.RunRecord; own, other string }{
  {first, "Market", "Event"}, {second, "Event", "Market"},
 } {
  h := mkHandler(t, &fakeRuns{rec:tc.rec}, application.ModeDecision)
  path := "/api/v1/research/runs/" + tc.rec.ID + "/stream"
  stream := apiRequest(t,h,http.MethodGet,path,"",true,map[string]string{"Last-Event-ID":"1"})
  if stream.Code != http.StatusOK || !strings.Contains(stream.Body.String(),"id: 2\n") ||
   !strings.Contains(stream.Body.String(),tc.own) || strings.Contains(stream.Body.String(),tc.other) {
   t.Fatalf("wrong Run replay for %s: %d %s",tc.rec.ID,stream.Code,stream.Body.String())
  }
  otherID := first.ID
  if tc.rec.ID == first.ID { otherID = second.ID }
  missing := apiRequest(t,h,http.MethodGet,"/api/v1/research/runs/"+otherID+"/stream","",true,nil)
  if missing.Code != http.StatusNotFound || strings.Contains(missing.Body.String(),tc.own) {
   t.Fatalf("cross-Run stream leaked event data: %d %s",missing.Code,missing.Body.String())
  }
 }
}

// Exercise a real HTTP socket lifecycle rather than merely invoking a recorder.
// The first connection is dropped after seq=1; the second replays 2..3.
func TestSSEHTTPDisconnectAndResumeWithoutDuplicate(t *testing.T) {
 record := testRecord()
 record.State = application.RunRunning
 record.FinishedAt = nil
 record.Events = []application.RunEvent{{Seq: 1, Stage: "Run", Status: "RUNNING", At: record.CreatedAt}}
 runs := &fakeRuns{rec: record}
 h := mkHandler(t, runs, application.ModeDecision)
 server := httptest.NewServer(h)
 defer server.Close()
 client := &http.Client{Timeout: 3 * time.Second}
 endpoint := server.URL + "/api/v1/research/runs/" + record.ID + "/stream"
 firstCtx, cancel := context.WithCancel(context.Background())
 req, err := http.NewRequestWithContext(firstCtx, http.MethodGet, endpoint, nil)
 if err != nil { t.Fatal(err) }
 req.Header.Set("Authorization", "Bearer " + testToken)
 res, err := client.Do(req)
 if err != nil { cancel(); t.Fatal(err) }
 reader := bufio.NewScanner(res.Body)
 if !reader.Scan() || reader.Text() != "id: 1" {
  cancel(); res.Body.Close(); t.Fatalf("initial SSE event missing: %s", reader.Text())
 }
 cancel()
 res.Body.Close()
 runs.mu.Lock()
 runs.rec.Events = append(runs.rec.Events,
  application.RunEvent{Seq: 2, Stage: "Event", Status: "DONE", At: record.CreatedAt},
  application.RunEvent{Seq: 3, Stage: "Run", Status: "COMPLETED", At: record.CreatedAt})
 runs.rec.State = application.RunCompleted
 finished := record.CreatedAt
 runs.rec.FinishedAt = &finished
 runs.mu.Unlock()
 replay, err := http.NewRequest(http.MethodGet, endpoint, nil)
 if err != nil { t.Fatal(err) }
 replay.Header.Set("Authorization", "Bearer " + testToken)
 replay.Header.Set("Last-Event-ID", "1")
 resumed, err := client.Do(replay)
 if err != nil { t.Fatal(err) }
 defer resumed.Body.Close()
 body, err := io.ReadAll(resumed.Body)
 if err != nil { t.Fatal(err) }
 data := string(body)
 if resumed.StatusCode != http.StatusOK ||
  strings.Contains(data, "id: 1\n") ||
  strings.Count(data, "id: 2\n") != 1 ||
  strings.Count(data, "id: 3\n") != 1 {
  t.Fatalf("SSE resume duplicate or missing events: %d %s", resumed.StatusCode, data)
 }
}
