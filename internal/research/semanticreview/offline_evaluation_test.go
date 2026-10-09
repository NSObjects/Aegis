package semanticreview

import (
	"reflect"
	"strings"
	"testing"
)

func evalInt64(x int64) *int64 { return &x }

func evaluationFixture(provenance DatasetProvenance) (*EvaluationDataset, *EvaluationRun) {
	newSample := func(id, argument string, expected Verdict, reviewers []string) EvaluationSample {
		return EvaluationSample{
			ID: id,
			Tasks: &Tasks{
				Symbol: "FICTIONAL.RESEARCH",
				Items: []Task{{InsightIndex: 0, ArgumentID: argument}},
			},
			Gold: []GoldLabel{{InsightIndex: 0, ArgumentID: argument, ExpectedVerdict: expected}},
			EvidenceRef: "fictional-test-evidence/" + id,
			AnnotatorIDs: reviewers,
		}
	}
	annotators := []string{"mock-reviewer-a", "mock-reviewer-b"}
	dataset := &EvaluationDataset{
		Version: "mock-dataset-v1",
		Provenance: provenance,
		Samples: []EvaluationSample{
			newSample("case-a", "arg-a", VerdictConsistent, annotators),
			newSample("case-b", "arg-b", VerdictContradictory, annotators),
			newSample("case-c", "arg-c", VerdictUncertain, annotators),
		},
	}
	makeReport := func(id string, verdict Verdict, uncertainty string) *Report {
		return &Report{
			Symbol: "FICTIONAL.RESEARCH",
			Findings: []Finding{{
				InsightIndex: 0, ArgumentID: id, Verdict: verdict,
				Rationale: "test-only independent verdict", Limitations: uncertainty,
			}},
		}
	}
	run := &EvaluationRun{
		DatasetVersion: "mock-dataset-v1", RunID: "offline-run-1", ModelID: "test-model",
		Samples: []EvaluationPrediction{
			{SampleID: "case-a", Report: makeReport("arg-a", VerdictConsistent, ""), LatencyMs: evalInt64(150), InputTokens: evalInt64(50), OutputTokens: evalInt64(10)},
			{SampleID: "case-b", Report: makeReport("arg-b", VerdictConsistent, ""), LatencyMs: evalInt64(350)},
			{SampleID: "case-c", FailureCode: FailureTimeout},
		},
	}
	return dataset,run
}

func TestOfflineBatchEvaluationSeparatesFailureFromUncertaintyAndCountsMeasuredOnly(t *testing.T) {
	set,run := evaluationFixture(ProvenanceSynthetic)
	original := append([]EvaluationPrediction(nil), run.Samples...)
	got,err := EvaluateRun(set,run)
	if err!=nil {t.Fatal(err)}
	if got.TotalSamples!=3 || got.CompletedSamples!=2 || got.UnavailableSamples!=1 ||
		got.LabeledItems!=3 || got.EvaluatedItems!=2 ||
		got.ExactMatches!=1 || got.FalseConsistentOnContradictory!=1 ||
		got.PredictedUncertain!=0 || got.ObservedLatencyCount!=2 ||
		got.P95LatencyMs==nil || *got.P95LatencyMs!=350 ||
		got.ObservedTokenCount!=1 || got.ObservedInputTokens!=50 || got.ObservedOutputTokens!=10 {
		t.Fatalf("incorrect batch metrics: %+v",got)
	}
	if !reflect.DeepEqual(original,run.Samples){t.Fatal("metrics mutated caller's prediction run")}
	if got.TotalSamples==0||got.EvaluatedItems==0 {t.Fatal("missing evaluation coverage")}
	if got.Provenance!=ProvenanceSynthetic {t.Fatal("synthetic provenance discarded")}
}

func TestOfflineBatchRejectsUntrustedCoverageAndVersionMismatches(t *testing.T){
	for _,tc:=range []struct{name string; change func(*EvaluationDataset,*EvaluationRun); want string}{
		{"bad version",func(_ *EvaluationDataset,r *EvaluationRun){r.DatasetVersion="other"},"version"},
		{"missing sample",func(_ *EvaluationDataset,r *EvaluationRun){r.Samples=r.Samples[:2]},"count"},
		{"reordered sample",func(_ *EvaluationDataset,r *EvaluationRun){r.Samples[0],r.Samples[1]=r.Samples[1],r.Samples[0]},"sample_id"},
		{"duplicate sample ids",func(_ *EvaluationDataset,r *EvaluationRun){r.Samples[1].SampleID="case-a"},"sample_id"},
		{"report and failure both supplied",func(_ *EvaluationDataset,r *EvaluationRun){r.Samples[0].FailureCode=FailureModelError},"both"},
		{"missing verdict and failure",func(_ *EvaluationDataset,r *EvaluationRun){r.Samples[0].Report=nil},"missing"},
		{"unknown failure code",func(_ *EvaluationDataset,r *EvaluationRun){r.Samples[2].FailureCode=FailureCode("UNKNOWN")},"failure"},
		{"negative latency",func(_ *EvaluationDataset,r *EvaluationRun){r.Samples[0].LatencyMs=evalInt64(-5)},"latency"},
		{"partial token metrics",func(_ *EvaluationDataset,r *EvaluationRun){r.Samples[0].InputTokens=nil},"token"},
		{"forged report identity",func(_ *EvaluationDataset,r *EvaluationRun){r.Samples[0].Report.Findings[0].ArgumentID="invented"},"argument_id"},
		{"invalid gold verdict",func(s *EvaluationDataset,_ *EvaluationRun){s.Samples[0].Gold[0].ExpectedVerdict=Verdict("CERTAIN")},"verdict"},
		{"extra prediction result",func(_ *EvaluationDataset,r *EvaluationRun){r.Samples=append(r.Samples,r.Samples[0])},"count"},
	}{
		t.Run(tc.name,func(t *testing.T){
			set,run:=evaluationFixture(ProvenanceSynthetic)
			tc.change(set,run)
			got,err:=EvaluateRun(set,run)
			if got!=nil||err==nil||!strings.Contains(strings.ToLower(err.Error()),strings.ToLower(tc.want)){
				t.Fatalf("accepted bad run: %+v error=%v, want=%q",got,err,tc.want)
			}
		})
	}
}

func TestOfflineGateNeverApprovesSyntheticOrUnreviewedData(t *testing.T){
	set,run:=evaluationFixture(ProvenanceSynthetic)
	result,err:=EvaluateRun(set,run)
	if err!=nil{t.Fatal(err)}
	policy:=QualityPolicy{
		MinLabeledItems:2,
		MinAvailabilityBps:5000,
		MinExactMatchBps:5000,
		MaxFalseConsistentOnContradictory:1,
	}
	out,err:=CheckQuality(set,run,policy)
	if err!=nil||out.Status!=QualityNotEligible {
		t.Fatalf("synthetic labels must never pass quality gate: %+v err=%v",out,err)
	}
	set.Provenance=ProvenanceHumanReviewed
	set.Samples[0].AnnotatorIDs=[]string{"only-one"}
	if result,err=EvaluateRun(set,run);err==nil||result!=nil{
		t.Fatalf("human-review metadata with one reviewer accepted: %+v %v",result,err)
	}
	set,run=evaluationFixture(ProvenanceHumanReviewed)
	set.Samples[0].EvidenceRef=""
	if result,err=EvaluateRun(set,run);err==nil||result!=nil{
		t.Fatalf("missing human-review evidence ref accepted: %+v %v",result,err)
	}
}

func TestOfflineGateBoundaryAndDangerousFalseAgreement(t *testing.T) {
	set,run:=evaluationFixture(ProvenanceHumanReviewed)
	// These metadata values are fictional test fixtures, NOT evidence that
	// real human annotations or real provider evaluations exist.
	result,err:=EvaluateRun(set,run)
	if err!=nil{t.Fatal(err)}
	policy:=QualityPolicy{
		MinLabeledItems:3,MinAvailabilityBps:6666,
		MinExactMatchBps:5000,MaxFalseConsistentOnContradictory:0,
	}
	bad,err:=CheckQuality(set,run,policy)
	if err!=nil||bad.Status!=QualityFail||len(bad.Reasons)==0 {
		t.Fatalf("hazardous false-consistency must fail: %+v %v",bad,err)
	}
	policy.MaxFalseConsistentOnContradictory=1
	good,err:=CheckQuality(set,run,policy)
	if err!=nil||good.Status!=QualityPass {
		t.Fatalf("policy boundary should pass mock metrics: %+v %v",good,err)
	}
	policy.MinAvailabilityBps=6667
	low,err:=CheckQuality(set,run,policy)
	if err!=nil||low.Status!=QualityFail{t.Fatalf("availability boundary incorrect: %+v %v",low,err)}
	policy.MinAvailabilityBps=0
	policy.MinLabeledItems=4
	ineligible,err:=CheckQuality(set,run,policy)
	if err!=nil||ineligible.Status!=QualityNotEligible{t.Fatalf("small samples must not pass: %+v %v",ineligible,err)}
	policy.MinLabeledItems=2
	policy.MinExactMatchBps=10001
	if _,err=CheckQuality(set,run,policy);err==nil{t.Fatal("invalid policy range accepted")}
}
