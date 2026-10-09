package research

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/semanticreview"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/risk"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type fullSemanticReviewerStub struct {
	calls int
	tasks *semanticreview.Tasks
	run func(context.Context,*semanticreview.Tasks)(*semanticreview.Report,error)
}
func (r *fullSemanticReviewerStub) Review(ctx context.Context,tasks *semanticreview.Tasks)(*semanticreview.Report,error){
	r.calls++
	r.tasks=tasks
	return r.run(ctx,tasks)
}

func semanticFullFixture(t *testing.T)(*FullPortfolioWorkflow,*reviewTraderStub,*reviewRiskStub,*portfolio.Context){
	t.Helper()
	graph,err:=NewArtifact(context.Background(),Config{
		MarketAgent:&fakeMarketAgent{result:validMarketContext()},
		TechnicalAgent:&fakeTechnicalAgent{result:validTechnicalContext()},
		EventAgent:&fakeEventAgent{result:validEventContext()},
		Synthesizer:&fakeSynthesizer{},
	})
	if err!=nil{t.Fatal(err)}
	review,_,mgr,trader,riskAnalyst:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
	mgr.plan.Opportunities=[]manager.Insight{{
		Claim:"technical upside remains possible",
		References:[]synthesis.Reference{{Domain:synthesis.DomainTechnical}},
		ArgumentIDs:[]string{"bull-1"},
	}}
	book:=reviewBookFixture()
	book.AsOf=validWorkflowInput().RequestedAsOf.Format(time.RFC3339)
	return &FullPortfolioWorkflow{Research:graph,Review:review},trader,riskAnalyst,book
}

func TestFullPortfolioSemanticReviewExplicitOptInLeavesCoreResultUnchanged(t *testing.T) {
	defaultFlow,_,_,defaultBook:=semanticFullFixture(t)
	baseline,err:=defaultFlow.Run(context.Background(),validWorkflowInput(),defaultBook)
	if err!=nil{t.Fatal(err)}
	oldJSON,err:=json.Marshal(baseline)
	if err!=nil{t.Fatal(err)}
	if strings.Contains(string(oldJSON),"semantic_review"){t.Fatalf("default JSON changed: %s",oldJSON)}

	enabled,trader,riskAnalyst,book:=semanticFullFixture(t)
	stub:=&fullSemanticReviewerStub{}
	stub.run=func(_ context.Context,tasks *semanticreview.Tasks)(*semanticreview.Report,error){
		if trader.calls!=1||riskAnalyst.calls!=1 {
			t.Fatalf("advisory review ran before core decisions: trader=%d risk=%d",trader.calls,riskAnalyst.calls)
		}
		if len(tasks.Items)!=1 || tasks.Items[0].ArgumentID!="bull-1"{
			t.Fatalf("wrong grounded review tasks: %+v",tasks)
		}
		return &semanticreview.Report{Symbol:tasks.Symbol,Findings:[]semanticreview.Finding{{
			InsightIndex:tasks.Items[0].InsightIndex,
			ArgumentID:tasks.Items[0].ArgumentID,
			Verdict:semanticreview.VerdictUncertain,
			Rationale:"cannot establish entailment",
			Limitations:"limited supporting claims",
		}}},nil
	}
	enabled.SemanticReviewer=stub
	got,err:=enabled.Run(context.Background(),validWorkflowInput(),book)
	if err!=nil{t.Fatal(err)}
	if stub.calls!=1||got.SemanticReview==nil||
		got.SemanticReview.Status!=SemanticReviewCompleted||
		got.SemanticReview.Report==nil||got.SemanticReview.TaskCount!=1{
		t.Fatalf("optional review not surfaced: %+v calls=%d",got.SemanticReview,stub.calls)
	}
	got.SemanticReview=nil
	newJSON,err:=json.Marshal(got)
	if err!=nil{t.Fatal(err)}
	if string(oldJSON)!=string(newJSON){
		t.Fatalf("advisory reviewer changed pre-existing workflow result: before=%s after=%s",oldJSON,newJSON)
	}
}

func TestFullPortfolioSemanticReviewUnavailableDoesNotInventFindingsOrChangeDecision(t *testing.T){
	for _,tc:=range []struct{
		name string
		run func(context.Context,*semanticreview.Tasks)(*semanticreview.Report,error)
	}{
		{"provider error",func(context.Context,*semanticreview.Tasks)(*semanticreview.Report,error){return nil,errors.New("sensitive provider error")}},
		{"malformed report",func(_ context.Context,tasks *semanticreview.Tasks)(*semanticreview.Report,error){
			return &semanticreview.Report{Symbol:tasks.Symbol,Findings:[]semanticreview.Finding{{
				InsightIndex:0,ArgumentID:"forged-id",Verdict:semanticreview.VerdictConsistent,Rationale:"hallucinated",
			}}},nil
		}},
		{"nil report",func(context.Context,*semanticreview.Tasks)(*semanticreview.Report,error){return nil,nil}},
	}{
		t.Run(tc.name,func(t *testing.T){
			w,trader,riskAnalyst,book:=semanticFullFixture(t)
			stub:=&fullSemanticReviewerStub{run:tc.run}
			w.SemanticReviewer=stub
			result,err:=w.Run(context.Background(),validWorkflowInput(),book)
			if err!=nil{t.Fatalf("independent reviewer broke completed research: %v",err)}
			if result==nil||result.PortfolioReview==nil||result.PortfolioReview.PortfolioDecision==nil||
				trader.calls!=1||riskAnalyst.calls!=1||stub.calls!=1{
				t.Fatalf("core decisions were changed or skipped: %+v",result)
			}
			audit:=result.SemanticReview
			if audit==nil||audit.Status!=SemanticReviewUnavailable||
				audit.Report!=nil||audit.TaskCount!=1||audit.ReasonCode==""{
				t.Fatalf("failed reviewer fabricated result: %+v",audit)
			}
			serialized,_:=json.Marshal(result)
			if strings.Contains(string(serialized),"sensitive provider error")||
				strings.Contains(string(serialized),"hallucinated"){
				t.Fatalf("provider output leaked in response: %s",serialized)
			}
		})
	}
}

func TestFullPortfolioSemanticReviewNoLinksSkipsReviewer(t *testing.T){
	w,_,_,book:=semanticFullFixture(t)
	// Manager returns a valid plan but does not authorize links for review.
	managerStub:=w.Review.cfg.ResearchManager.(*reviewManagerStub)
	managerStub.plan.Opportunities[0].ArgumentIDs=nil
	stub:=&fullSemanticReviewerStub{run:func(context.Context,*semanticreview.Tasks)(*semanticreview.Report,error){
		return nil,errors.New("must not call an external model without explicit links")
	}}
	w.SemanticReviewer=stub
	result,err:=w.Run(context.Background(),validWorkflowInput(),book)
	if err!=nil{t.Fatal(err)}
	if result.SemanticReview==nil||result.SemanticReview.Status!=SemanticReviewNoLinks||
		result.SemanticReview.Report!=nil||result.SemanticReview.TaskCount!=0||stub.calls!=0 {
		t.Fatalf("no-link handling is unsafe: %+v calls=%d",result.SemanticReview,stub.calls)
	}
}
