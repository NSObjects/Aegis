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
