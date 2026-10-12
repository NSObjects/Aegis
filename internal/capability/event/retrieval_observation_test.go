package event

import (
 "context"
 "errors"
 "testing"
)

func TestAcquisitionRunnerObservesExecutedRetrieversOnly(t *testing.T) {
 var got []RetrievalObservation
 ctx := WithRetrievalObserver(context.Background(), func(ev RetrievalObservation) { got = append(got, ev) })
 calls := 0
 _, err := (AcquisitionPlanRunner{}).Run(ctx,
  ResearchTask{ID:"observable-event", Symbol:"600519.SH", Objective:"evidence"},
  AcquisitionPlan{Steps: []AcquisitionStep{{
   ID:"official", Scope:AcquisitionScopeDirect,
   Retriever:countingRetriever{id:"snapshot_official",method:AcquisitionAPI,err:errors.New("secret provider error"),callCount:&calls},
  }}})
 if err != nil { t.Fatalf("retriever failure should be reported through task output, got %v",err) }
 if calls != 1 { t.Fatalf("retriever calls = %d, want 1",calls) }
 if len(got) != 2 || got[0].Status != "START" || got[1].Status != "DONE" {
  t.Fatalf("observations = %+v",got)
 }
 for _, observation := range got {
  if observation.RetrieverID != "snapshot_official" || observation.At.IsZero() {
   t.Fatalf("unexpected observation %+v",observation)
  }
 }
 if got[1].DurationMS < 0 {t.Fatal("negative duration")}
}
