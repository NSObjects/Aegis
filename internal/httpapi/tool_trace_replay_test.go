package httpapi

import (
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
