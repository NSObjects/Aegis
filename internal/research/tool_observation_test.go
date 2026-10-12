package research

import (
 "context"
 "testing"
 "time"
)

func TestToolObservationsRejectUnsafeData(t *testing.T) {
 var got []ToolObservation
 ctx := WithToolObservations(context.Background(), func(ev ToolObservation) { got = append(got, ev) })
 for _, ev := range []ToolObservation{
  {Stage:"Event",Tool:"news_search",Status:"START"},
  {Stage:"Event",Tool:"news_search",Status:"DONE",DurationMS:0},
  {Stage:"Event",Tool:"api_key=secret",Status:"DONE"},
  {Stage:"Unexpected",Tool:"news_search",Status:"DONE"},
  {Stage:"Event",Tool:"news_search",Status:"whatever"},
  {Stage:"Event",Tool:"news_search",Status:"FAIL",DurationMS:-1},
 } { ReportToolObservation(ctx, ev) }
 if len(got) != 2 { t.Fatalf("expected two safe observations, got %+v", got) }
 if got[0].At.IsZero() || got[0].At.After(time.Now().Add(time.Second)) { t.Fatalf("timestamp missing: %+v",got[0]) }
 if got[1].Tool != "news_search" || got[1].Status != "DONE" { t.Fatalf("wrong terminal event: %+v",got[1]) }
 ReportToolObservation(context.Background(), ToolObservation{Stage:"Event",Tool:"news_search",Status:"DONE"})
}
