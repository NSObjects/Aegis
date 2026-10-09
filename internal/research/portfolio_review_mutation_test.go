package research

import (
	"Aegis/internal/research/manager"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
)

type mutatingReviewTrader struct {
	intent *tradeintent.Intent
	mutate func(*synthesis.Artifact, *manager.ResearchPlan)
}

func (s *mutatingReviewTrader) AnalyzeResearchPlanIntent(
	_ context.Context, artifact *synthesis.Artifact, plan *manager.ResearchPlan, _ ...adk.AgentRunOption,
) (*tradeintent.Intent, error) {
	s.mutate(artifact, plan)
	return s.intent, nil
}

func TestPortfolioReviewFailsClosedIfTradingMutatesResearch(t *testing.T) {
	cases := []struct {
		name string
		mutate func(*synthesis.Artifact, *manager.ResearchPlan)
	}{
		{"disposition", func(_ *synthesis.Artifact, p *manager.ResearchPlan) {
			p.AcceptedArgumentIDs = []string{"bear-1"}
			p.UnresolvedArgumentIDs = []string{"bull-1"}
		}},
		{"research summary", func(_ *synthesis.Artifact, p *manager.ResearchPlan) {
			p.Summary = "changed after validation"
		}},
		{"artifact summary", func(a *synthesis.Artifact, _ *manager.ResearchPlan) {
			a.Synthesis.Summary = "changed after validation"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, _, _, _, riskAgent := reviewWorkflowFixture(t, reviewContextFixture(risk.AssessmentHighRisk))
			w.cfg.TradingAgent = &mutatingReviewTrader{intent: reviewIntentFixture(), mutate: tc.mutate}
			got, err := w.Run(context.Background(), reviewArtifactFixture(), reviewBookFixture())
			if got != nil || err == nil || !strings.Contains(err.Error(), "mutated validated research inputs") {
				t.Fatalf("must reject changed inputs before Risk: result=%+v err=%v", got, err)
			}
			if riskAgent.calls != 0 {
				t.Fatalf("risk called after mutated research: %d", riskAgent.calls)
			}
		})
	}
}
