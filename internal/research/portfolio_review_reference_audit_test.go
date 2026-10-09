package research

import (
	"Aegis/internal/research/evidenceledger"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/risk"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPortfolioReviewExposesAuditWithoutChangingDecision(t *testing.T) {
	w,_,mgr,trader,riskAnalyst:=reviewWorkflowFixture(t,reviewContextFixture(risk.AssessmentHighRisk))
	mgr.plan.Opportunities=[]manager.Insight{{
		Claim:"technical possibility",
		References:[]synthesis.Reference{{Domain:synthesis.DomainTechnical}},
		ArgumentIDs:[]string{"bear-1","bull-1"},
	}}
	result,err:=w.Run(context.Background(),reviewArtifactFixture(),reviewBookFixture())
	if err!=nil {t.Fatal(err)}
	want:=[]evidenceledger.ArgumentReferenceAudit{
		{ArgumentID:"bear-1",ReferenceAlignment:evidenceledger.AlignmentNoSharedReference},
		{ArgumentID:"bull-1",ReferenceAlignment:evidenceledger.AlignmentSameDomainOnly},
	}
	if result.EvidenceLedger==nil || len(result.EvidenceLedger.Insights)!=1 {
		t.Fatalf("missing Ledger: %+v",result)
	}
	audits:=result.EvidenceLedger.Insights[0].ArgumentReferenceAudits
	if len(audits)!=len(want){t.Fatalf("audits=%+v want=%+v",audits,want)}
	for i:=range want{if audits[i]!=want[i]{t.Fatalf("audit %d=%+v want=%+v",i,audits[i],want[i])}}
	if trader.calls!=1 || riskAnalyst.calls!=1 || result.PortfolioDecision==nil {
		t.Fatalf("audit must not block or replace downstream stages: trading=%d risk=%d result=%+v",trader.calls,riskAnalyst.calls,result)
	}
	encoded,err:=json.Marshal(result)
	if err!=nil {t.Fatal(err)}
	if !strings.Contains(string(encoded),`"argument_reference_audits":[{"argument_id":"bear-1","reference_alignment":"NO_SHARED_REFERENCE"}`) {
		t.Fatalf("missing structured audit in PortfolioReview JSON: %s",encoded)
	}
}
