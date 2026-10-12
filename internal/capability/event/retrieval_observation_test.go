package event

import (
 "context"
 "errors"
 "testing"
 "strings"
 "time"
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
 if len(got) != 2 || got[0].Status != "START" || got[1].Status != "FAIL" {
  t.Fatalf("observations = %+v",got)
 }
 for _, observation := range got {
  if observation.RetrieverID != "snapshot_official" || observation.At.IsZero() {
   t.Fatalf("unexpected observation %+v",observation)
  }
 }
 if got[1].DurationMS < 0 {t.Fatal("negative duration")}
}

func TestAcquisitionRunnerFiveSuccessfulToolsCanFailDuringEvidenceSelection(t *testing.T) {
 // Regression for the observation pattern seen on failed Event Run
 // 7adfc261750369b23c9f1d981559e170. This deliberately simulates a
 // post-retrieval failure; it does not claim the historical root cause.
 published := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
 names := []string{"cninfo_announcements", "cls_news_telegraph", "tavily_web_search", "snapshot_official", "snapshot_breaking_news"}
 steps := make([]AcquisitionStep, 0, len(names))
 for i, name := range names {
  scope := AcquisitionScopeDirect
  if i == 2 { scope = AcquisitionScopeCandidate }
  steps = append(steps, AcquisitionStep{
   ID: name, Scope: scope,
   Retriever: countingRetriever{
    id: name, method: AcquisitionAPI,
    docs: []RawDocument{{
     ID: name + "_document", Title: "公司事件证据", Content: "贵州茅台相关事件",
     URL: "https://example.org/" + name, Publisher: name,
     SourceType: SourceIndustryMedia, PublishedAt: &published,
    }},
   },
  })
 }
 var observed []RetrievalObservation
 ctx := WithRetrievalObserver(context.Background(), func(ev RetrievalObservation) { observed = append(observed, ev) })
 output, err := (AcquisitionPlanRunner{}).Run(ctx,
  ResearchTask{ID:"five-tool-postprocessing", Symbol:"600519.SH", Objective:"event evidence"},
  AcquisitionPlan{Steps:steps},
 )
 if err == nil || !strings.Contains(err.Error(), "event evidence selector is required") {
  t.Fatalf("expected failure after all tool calls, got %v", err)
 }
 if len(observed) != 10 { t.Fatalf("observations = %d, want 10",len(observed)) }
 for i, name := range names {
  start, done := observed[i*2], observed[i*2+1]
  if start.RetrieverID != name || start.Status != "START" ||
     done.RetrieverID != name || done.Status != "DONE" || done.Outcome != "SUCCEEDED" {
   t.Fatalf("tool %s observations = %+v %+v",name,start,done)
  }
 }
 if len(output.Steps) != 5 || len(output.CandidateRawDocuments) == 0 {
  t.Fatalf("expected five completed steps and candidate documents, got %d steps and %d candidates",
   len(output.Steps), len(output.CandidateRawDocuments))
 }
 if len(output.Canonicalization.Documents) != 0 {
  t.Fatal("post-processing failure must not be treated as successful root canonicalization")
 }
}
