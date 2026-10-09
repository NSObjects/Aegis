package semanticreview

import (
	"Aegis/internal/agent/event"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/research/debate"
	"Aegis/internal/research/evidenceledger"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/synthesis"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func semanticFixture() (*synthesis.Artifact, *debate.State, *manager.ResearchPlan) {
	artifact := &synthesis.Artifact{
		Symbol: "600519.SH",
		Market: &market.Context{AsOf: "20260930", Regime: market.RegimeMixed, Evidence: []string{"breadth"}, Confidence: .7},
		Technical: &technical.Context{Symbol: "600519.SH", RequestedAsOf: "20260930", EffectiveAsOf: "20260930", Status: technical.StatusNormal, Snapshot: &technical.Snapshot{Close: 100}, Evidence: []string{"technical momentum"}, Confidence: .8},
		Event: &event.Context{Symbol: "600519.SH", RequestedAsOf: "2026-09-30T15:00:00+08:00", Status: event.StatusNoEvent, Confidence: .5},
		Synthesis: synthesis.Synthesis{Summary: "mixed evidence", Confidence: .6},
	}
	state := &debate.State{Symbol: "600519.SH", Turns: []debate.Turn{
		{Round: 1, Side: debate.SideBull, Thesis: "constructive", Confidence: .7, Arguments: []debate.Argument{{ID: "bull-1", Claim: "technical momentum", References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}}}}},
		{Round: 1, Side: debate.SideBear, Thesis: "cautious", Confidence: .7, Arguments: []debate.Argument{{ID: "bear-1", Claim: "market weakness", References: []synthesis.Reference{{Domain: synthesis.DomainMarket}}}}},
	}}
	plan := &manager.ResearchPlan{
		Symbol: "600519.SH", Conclusion: manager.ConclusionBalanced, Summary: "both sides considered", Confidence: .6,
		AcceptedArgumentIDs: []string{"bull-1"}, UnresolvedArgumentIDs: []string{"bear-1"},
		Opportunities: []manager.Insight{{Claim: "potential technical setup", References: []synthesis.Reference{{Domain: synthesis.DomainTechnical}}, ArgumentIDs: []string{"bear-1", " bull-1 "}}},
		Risks: []manager.Insight{{Claim: "market weakness remains", References: []synthesis.Reference{{Domain: synthesis.DomainMarket}}, ArgumentIDs: []string{"bear-1"}}},
	}
	return artifact, state, plan
}

func TestBuildTasksPreservesRealLinksAndDoesNotInventVerdicts(t *testing.T) {
	artifact, state, plan := semanticFixture()
	got, err := BuildTasks(artifact, state, plan)
	if err != nil { t.Fatal(err) }
	if got.Symbol != "600519.SH" || len(got.Items) != 3 {
		t.Fatalf("unexpected task envelope: %+v", got)
	}
	want := []struct{ index int; id string; match evidenceledger.ReferenceAlignment }{
		{0,"bear-1",evidenceledger.AlignmentNoSharedReference},
		{0,"bull-1",evidenceledger.AlignmentSameDomainOnly},
		{1,"bear-1",evidenceledger.AlignmentSameDomainOnly},
	}
	for i, item := range got.Items {
		if item.InsightIndex != want[i].index || item.ArgumentID != want[i].id || item.ReferenceAlignment != want[i].match {
			t.Fatalf("task %d = %+v, want %+v", i, item, want[i])
		}
		if item.InsightClaim == "" || item.ArgumentClaim == "" ||
			len(item.InsightReferences) != 1 || len(item.ArgumentReferences) != 1 {
			t.Fatalf("task missing original claims or citations: %+v", item)
		}
	}
	initial, err := json.Marshal(got)
	if err != nil { t.Fatal(err) }
	if strings.Contains(string(initial), "verdict") || strings.Contains(string(initial), "rationale") {
		t.Fatalf("builder invented semantic conclusions: %s", initial)
	}
	repeated, err := BuildTasks(artifact,state,plan)
	if err != nil { t.Fatal(err) }
	next, _ := json.Marshal(repeated)
	if string(initial) != string(next) { t.Fatal("task output is not deterministic") }

	got.Items[0].InsightReferences[0].Domain = synthesis.DomainMarket
	got.Items[0].ArgumentReferences[0].Domain = synthesis.DomainEvent
	if plan.Opportunities[0].References[0].Domain != synthesis.DomainTechnical ||
		state.Turns[1].Arguments[0].References[0].Domain != synthesis.DomainMarket {
		t.Fatal("task references alias mutable sources")
	}
	if !reflect.DeepEqual(plan.Opportunities[0].ArgumentIDs, []string{"bear-1"," bull-1 "}) {
		t.Fatal("builder mutated IDs in plan")
	}
}

func TestBuildTasksUsesUpstreamValidationAndKeepsUnlinkedPlansEmpty(t *testing.T) {
	artifact,state,plan:=semanticFixture()
	plan.Opportunities[0].ArgumentIDs=nil
	plan.Risks[0].ArgumentIDs=nil
	got,err:=BuildTasks(artifact,state,plan)
	if err != nil || got==nil || len(got.Items)!=0 {
		t.Fatalf("no explicit links should produce no review tasks: got=%+v err=%v",got,err)
	}
	plan.Opportunities[0].ArgumentIDs=[]string{"fabricated"}
	got,err=BuildTasks(artifact,state,plan)
	if got!=nil || err==nil || !strings.Contains(err.Error(),"does not exist in debate") {
		t.Fatalf("invalid provenance should fail closed: got=%+v err=%v",got,err)
	}
	artifact,state,plan=semanticFixture()
	got,err=BuildTasks(nil,state,plan)
	if got!=nil || err==nil {t.Fatalf("nil artifact accepted: %+v %v",got,err)}
}

func TestValidateReportRejectsInventedOrUnsupportedOpinions(t *testing.T) {
	tasks := &Tasks{Symbol:"600519.SH",Items:[]Task{{InsightIndex:0,ArgumentID:"bear-1"},{InsightIndex:0,ArgumentID:"bull-1"}}}
	newReport := func() *Report {
		return &Report{Symbol:"600519.SH",Findings:[]Finding{
			{InsightIndex:0,ArgumentID:"bear-1",Verdict:VerdictUncertain,Rationale:"conflicting evidence",Limitations:"insufficient specific facts"},
			{InsightIndex:0,ArgumentID:"bull-1",Verdict:VerdictConsistent,Rationale:"claims appear consistent but need human scrutiny"},
		}}
	}
	if err:=ValidateReport(tasks,newReport());err!=nil {t.Fatalf("valid advisory report: %v",err)}
	tests:=[]struct {
		name string
		edit func(*Report)
		want string
	}{
		{"wrong symbol",func(r *Report){r.Symbol="000001.SZ"},"symbol"},
		{"missing finding",func(r *Report){r.Findings=r.Findings[:1]},"count"},
		{"invented argument",func(r *Report){r.Findings[1].ArgumentID="invented"},"argument_id"},
		{"duplicate argument",func(r *Report){r.Findings[1].ArgumentID="bear-1"},"argument_id"},
		{"reordered results",func(r *Report){r.Findings[0],r.Findings[1]=r.Findings[1],r.Findings[0]},"argument_id"},
		{"unsupported verdict",func(r *Report){r.Findings[1].Verdict=Verdict("CERTAIN")},"verdict"},
		{"missing rationale",func(r *Report){r.Findings[1].Rationale="  "},"rationale"},
		{"missing uncertain limitation",func(r *Report){r.Findings[0].Limitations=" "},"limitations"},
	}
	for _,tc:=range tests{t.Run(tc.name,func(t *testing.T){
		report:=newReport()
		tc.edit(report)
		err:=ValidateReport(tasks,report)
		if err==nil || !strings.Contains(err.Error(),tc.want){t.Fatalf("report=%+v err=%v, want %q",report,err,tc.want)}
	})}
	if err:=ValidateReport(tasks,nil);err==nil{t.Fatal("nil report accepted")}
	if err:=ValidateReport(nil,newReport());err==nil{t.Fatal("nil tasks accepted")}
	if err:=ValidateReport(&Tasks{Symbol:"600519.SH"},&Report{Symbol:"600519.SH"});err!=nil{t.Fatalf("empty review is allowed but cannot invent a verdict: %v",err)}
}
