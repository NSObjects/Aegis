package semanticreview

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSyntheticGoldCorpusAndAgreementScorer(t *testing.T){
	var corpus struct{
		Version string `json:"version"`
		Cases []struct{
			InsightIndex int `json:"insight_index"`
			ArgumentID string `json:"argument_id"`
			InsightClaim string `json:"insight_claim"`
			ArgumentClaim string `json:"argument_claim"`
			ReferenceAlignment string `json:"reference_alignment"`
			ExpectedVerdict Verdict `json:"expected_verdict"`
			Annotation string `json:"annotation"`
		} `json:"cases"`
	}
	data,err:=os.ReadFile("testdata/gold_v1.json")
	if err!=nil{t.Fatal(err)}
	if err:=json.Unmarshal(data,&corpus);err!=nil{t.Fatal(err)}
	if corpus.Version!="synthetic-v1"||len(corpus.Cases)<6{t.Fatalf("missing versioned test corpus: %s",data)}
	tasks:=&Tasks{Symbol:"FICTIONAL.RESEARCH"}
	report:=&Report{Symbol:tasks.Symbol}
	labels:=make([]GoldLabel,0,len(corpus.Cases))
	for _,example:=range corpus.Cases {
		if example.InsightClaim==""||example.ArgumentClaim==""||example.Annotation==""{
			t.Fatalf("unexplained synthetic example: %+v",example)
		}
		tasks.Items=append(tasks.Items,Task{InsightIndex:example.InsightIndex,ArgumentID:example.ArgumentID})
		finding:=Finding{
			InsightIndex:example.InsightIndex,
			ArgumentID:example.ArgumentID,
			Verdict:example.ExpectedVerdict,
			Rationale:"synthetic test gold label only",
		}
		if example.ExpectedVerdict==VerdictUncertain{finding.Limitations="synthetic examples cannot establish factual truth"}
		report.Findings=append(report.Findings,finding)
		labels=append(labels,GoldLabel{
			InsightIndex:example.InsightIndex,ArgumentID:example.ArgumentID,ExpectedVerdict:example.ExpectedVerdict,
		})
	}
	score,err:=ScoreLabeledReport(tasks,report,labels)
	if err!=nil{t.Fatal(err)}
	if score.Total!=len(labels)||score.ExactMatches!=score.Total||
		score.FalseConsistentOnContradictory!=0||score.FalseContradictoryOnConsistent!=0||
		score.PredictedUncertain!=2 {
		t.Fatalf("perfect synthetic label agreement was not scored correctly: %+v",score)
	}

	// The scorer must not mistake domain or same-event overlap for correct
	// predictions. Two intentionally incorrect classifications are counted.
	report.Findings[4].Verdict=VerdictConsistent // gold is CONTRADICTORY
	report.Findings[2].Verdict=VerdictConsistent // gold is UNCERTAIN
	score,err=ScoreLabeledReport(tasks,report,labels)
	if err!=nil{t.Fatal(err)}
	if score.Total!=8||score.ExactMatches!=6||
		score.FalseConsistentOnContradictory!=1||
		score.PredictedUncertain!=1 {
		t.Fatalf("misclassifications were missed: %+v",score)
	}

	labels[0].ArgumentID="fabricated"
	if _,err:=ScoreLabeledReport(tasks,report,labels);err==nil{
		t.Fatal("scorer accepted invented gold identity")
	}
}

func TestEvaluationScorerRejectsInvalidExternalReports(t *testing.T){
	tasks:=&Tasks{Symbol:"FICTIONAL.RESEARCH",Items:[]Task{{InsightIndex:0,ArgumentID:"id-1"}}}
	gold:=[]GoldLabel{{InsightIndex:0,ArgumentID:"id-1",ExpectedVerdict:VerdictContradictory}}
	report:=&Report{Symbol:tasks.Symbol,Findings:[]Finding{{
		InsightIndex:0,ArgumentID:"id-1",Verdict:VerdictConsistent,Rationale:"unverified opinion",
	}}}
	if _,err:=ScoreLabeledReport(tasks,report,nil);err==nil{t.Fatal("missing gold accepted")}
	invalid:=[]GoldLabel{{InsightIndex:0,ArgumentID:"id-1",ExpectedVerdict:Verdict("PROVEN")}}
	if _,err:=ScoreLabeledReport(tasks,report,invalid);err==nil{t.Fatal("unsupported gold verdict accepted")}
	report.Findings[0].ArgumentID="unknown"
	if _,err:=ScoreLabeledReport(tasks,report,gold);err==nil{t.Fatal("invalid report passed evaluation")}
}
