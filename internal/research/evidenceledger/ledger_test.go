package evidenceledger

import (
	"Aegis/internal/agent/event"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/research/debate"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/synthesis"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func ledgerFixture() (*synthesis.Artifact,*debate.State,*manager.ResearchPlan) {
	artifact:=&synthesis.Artifact{
		Symbol:"600519.SH",
		Market:&market.Context{AsOf:"20260930",Regime:market.RegimeMixed,Evidence:[]string{"market evidence"},Confidence:0.7},
		Technical:&technical.Context{Symbol:"600519.SH",RequestedAsOf:"20260930",EffectiveAsOf:"20260930",
			Status:technical.StatusNormal,Snapshot:&technical.Snapshot{Close:100},Evidence:[]string{"technical evidence"},Confidence:0.8},
		Event:&event.Context{Symbol:"600519.SH",RequestedAsOf:"2026-09-30T15:00:00+08:00",Status:event.StatusNoEvent,Confidence:0.5},
		Synthesis:synthesis.Synthesis{Summary:"mixed market and technical observations",Confidence:0.7},
	}
	state:=&debate.State{Symbol:"600519.SH",Turns:[]debate.Turn{
		{Round:1,Side:debate.SideBull,Thesis:"constructive",Confidence:0.7,Arguments:[]debate.Argument{
			{ID:"bull-1",Claim:"technical momentum",References:[]synthesis.Reference{{Domain:synthesis.DomainTechnical}}},
			{ID:"bull-2",Claim:"technical resilience",References:[]synthesis.Reference{{Domain:synthesis.DomainTechnical}}},
		}},
		{Round:1,Side:debate.SideBear,Thesis:"cautious",Confidence:0.7,Arguments:[]debate.Argument{
			{ID:"bear-1",Claim:"market weakness",References:[]synthesis.Reference{{Domain:synthesis.DomainMarket}}},
		},Rebuttals:[]debate.Rebuttal{
			{Claim:"a broad risk-off market matters",TargetArgumentID:"bull-1",
				References:[]synthesis.Reference{{Domain:synthesis.DomainMarket}}},
		}},
	}}
	plan:=&manager.ResearchPlan{
		Symbol:"600519.SH",Conclusion:manager.ConclusionBalanced,Summary:"competing conditions",
		AcceptedArgumentIDs:[]string{"bull-1"},UnresolvedArgumentIDs:[]string{"bear-1"},
		Opportunities:[]manager.Insight{{Claim:"tech setup",References:[]synthesis.Reference{{Domain:synthesis.DomainTechnical}}}},
		Risks:[]manager.Insight{{Claim:"market uncertainty",References:[]synthesis.Reference{{Domain:synthesis.DomainMarket}}}},
		Confidence:0.6,
	}
	return artifact,state,plan
}

func TestBuildGroundedLedgerWithAcceptedUnresolvedAndUnclassified(t *testing.T){
	artifact,state,plan:=ledgerFixture()
	beforeArtifact,_:=json.Marshal(artifact)
	beforeState,_:=json.Marshal(state)
	beforePlan,_:=json.Marshal(plan)
	got,err:=Build(artifact,state,plan)
	if err!=nil {t.Fatal(err)}
	if got.Symbol!="600519.SH" || got.Conclusion!=manager.ConclusionBalanced ||
		len(got.Arguments)!=3 || len(got.Insights)!=2 {
		t.Fatalf("missing grounded research items: %+v",got)
	}
	if got.Arguments[0].ID!="bull-1" || got.Arguments[0].Disposition!=DispositionAccepted ||
		got.Arguments[0].Side!=debate.SideBull || got.Arguments[0].Round!=1 ||
		len(got.Arguments[0].Challenges)!=1 ||
		got.Arguments[0].Challenges[0].Claim!="a broad risk-off market matters" ||
		got.Arguments[0].Challenges[0].Side!=debate.SideBear {
		t.Fatalf("accepted argument challenge provenance is missing: %+v",got.Arguments[0])
	}
	if got.Arguments[1].Disposition!=DispositionUnclassified ||
		got.Arguments[1].ID!="bull-2" || len(got.Arguments[1].Challenges)!=0 {
		t.Fatalf("an unmentioned argument is not automatically rejected: %+v",got.Arguments[1])
	}
	if got.Arguments[2].Disposition!=DispositionUnresolved || got.Arguments[2].ID!="bear-1" {
		t.Fatalf("unresolved debate argument lost: %+v",got.Arguments[2])
	}
	if got.Insights[0].Kind!=InsightOpportunity ||
		got.Insights[0].References[0].Domain!=synthesis.DomainTechnical ||
		got.Insights[1].Kind!=InsightRisk ||
		got.Insights[1].References[0].Domain!=synthesis.DomainMarket {
		t.Fatalf("plan insight provenance lost: %+v",got.Insights)
	}
	for i:=0;i<12;i++ {
		repeated,err:=Build(artifact,state,plan)
		if err!=nil {t.Fatal(err)}
		if !reflect.DeepEqual(repeated,got){t.Fatal("same input must preserve stable ledger order")}
		b1,_:=json.Marshal(got);b2,_:=json.Marshal(repeated)
		if string(b1)!=string(b2){t.Fatal("JSON output must be stable byte-for-byte")}
	}
	afterArtifact,_:=json.Marshal(artifact)
	afterState,_:=json.Marshal(state)
	afterPlan,_:=json.Marshal(plan)
	if string(beforeArtifact)!=string(afterArtifact) ||
		string(beforeState)!=string(afterState) ||
		string(beforePlan)!=string(afterPlan) {
		t.Fatal("Build mutated input ResearchArtifact, Debate or ResearchPlan")
	}
	// Ensure independent copies of the shared reference slices.
	got.Arguments[0].References[0].Domain=synthesis.DomainEvent
	got.Arguments[0].Challenges[0].References[0].Domain=synthesis.DomainEvent
	got.Insights[0].References[0].Domain=synthesis.DomainEvent
	if state.Turns[0].Arguments[0].References[0].Domain!=synthesis.DomainTechnical ||
		state.Turns[1].Rebuttals[0].References[0].Domain!=synthesis.DomainMarket ||
		plan.Opportunities[0].References[0].Domain!=synthesis.DomainTechnical {
		t.Fatal("ledger reference slices must not alias upstream inputs")
	}
}

func TestBuildRejectsBadArgumentMappings(t *testing.T) {
	tests:=[]struct{
		name string
		edit func(*synthesis.Artifact,*debate.State,*manager.ResearchPlan)
		want string
	}{
		{"unknown accepted",func(_ *synthesis.Artifact,_ *debate.State,p *manager.ResearchPlan){p.AcceptedArgumentIDs=[]string{"unknown"}},"does not exist in debate"},
		{"unknown unresolved",func(_ *synthesis.Artifact,_ *debate.State,p *manager.ResearchPlan){p.UnresolvedArgumentIDs=[]string{"unknown"}},"does not exist in debate"},
		{"duplicate accepted",func(_ *synthesis.Artifact,_ *debate.State,p *manager.ResearchPlan){p.AcceptedArgumentIDs=[]string{"bull-1","bull-1"}},"appears in both"},
		{"overlap dispositions",func(_ *synthesis.Artifact,_ *debate.State,p *manager.ResearchPlan){p.UnresolvedArgumentIDs=[]string{"bull-1"}},"appears in both"},
		{"duplicate debate ID",func(_ *synthesis.Artifact,s *debate.State,_ *manager.ResearchPlan){s.Turns[1].Arguments[0].ID="bull-1"},"already exists"},
		{"invented target",func(_ *synthesis.Artifact,s *debate.State,_ *manager.ResearchPlan){s.Turns[1].Rebuttals[0].TargetArgumentID="ghost"},"prior argument"},
		{"same-turn target",func(_ *synthesis.Artifact,s *debate.State,_ *manager.ResearchPlan){s.Turns[1].Rebuttals[0].TargetArgumentID="bear-1"},"prior argument"},
		{"bad plan symbol",func(_ *synthesis.Artifact,_ *debate.State,p *manager.ResearchPlan){p.Symbol="000001.SZ"},"does not match"},
		{"bad debate symbol",func(_ *synthesis.Artifact,s *debate.State,_ *manager.ResearchPlan){s.Symbol="000001.SZ"},"does not match"},
		{"invented event id",func(_ *synthesis.Artifact,s *debate.State,_ *manager.ResearchPlan){
			s.Turns[0].Arguments[0].References=[]synthesis.Reference{{Domain:synthesis.DomainEvent,EventID:"fake-event"}}
		},"does not exist"},
		{"wrong reference domain",func(_ *synthesis.Artifact,_ *debate.State,p *manager.ResearchPlan){
			p.Risks[0].References=[]synthesis.Reference{{Domain:synthesis.Domain("FUNDAMENTAL")}}
		},"invalid reference domain"},
		{"event id on market ref",func(_ *synthesis.Artifact,s *debate.State,_ *manager.ResearchPlan){
			s.Turns[0].Arguments[0].References[0].EventID="fake-event"
		},"cannot carry event_id"},
	}
	for _,tt:=range tests {
		t.Run(tt.name,func(t *testing.T){
			a,s,p:=ledgerFixture()
			tt.edit(a,s,p)
			got,err:=Build(a,s,p)
			if err==nil || got!=nil || !strings.Contains(err.Error(),tt.want) {
				t.Fatalf("wanted fail-closed error containing %q, got %+v, %v",tt.want,got,err)
			}
		})
	}
}

func TestBuildHandlesValidatorCanonicalIDsAndDomainLevelEvent(t *testing.T){
	a,s,p:=ledgerFixture()
	s.Turns[0].Arguments[0].ID=" bull-1 "
	s.Turns[1].Rebuttals[0].TargetArgumentID=" bull-1 "
	p.AcceptedArgumentIDs=[]string{" bull-1 "}
	p.Risks[0].References=[]synthesis.Reference{{Domain:synthesis.DomainEvent}} // domain-level; not an event ID
	out,err:=Build(a,s,p)
	if err!=nil {t.Fatal(err)}
	if out.Arguments[0].ID!="bull-1" || out.Arguments[0].Disposition!=DispositionAccepted ||
		len(out.Arguments[0].Challenges)!=1 ||
		out.Insights[1].References[0].EventID!="" {
		t.Fatalf("canonicalized links or domain-level event ref not preserved: %+v",out)
	}
}

func TestBuildRequiresValidNonEmptyDebateAndArtifact(t *testing.T){
	a,s,p:=ledgerFixture()
	s.Turns=nil
	got,err:=Build(a,s,p)
	if err==nil || got!=nil || !strings.Contains(err.Error(),"non-empty debate"){t.Fatalf("got %+v %v",got,err)}
	a,s,p=ledgerFixture()
	a.Market=nil
	got,err=Build(a,s,p)
	if err==nil || got!=nil || !strings.Contains(err.Error(),"market"){t.Fatalf("got %+v %v",got,err)}
}
