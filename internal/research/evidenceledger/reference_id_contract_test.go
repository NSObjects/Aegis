package evidenceledger

import (
	"Aegis/internal/research/debate"
	"Aegis/internal/research/manager"
	"Aegis/internal/research/synthesis"
	"strings"
	"testing"
)

func TestBuildRejectsNoncanonicalReferenceAtEachSource(t *testing.T) {
	cases := []struct {
		name string
		edit func(*synthesis.Artifact, *debate.State, *manager.ResearchPlan)
	}{
		{"bull argument", func(_ *synthesis.Artifact, s *debate.State, _ *manager.ResearchPlan) {
			s.Turns[0].Arguments[0].References[0].EventID = " "
		}},
		{"bear rebuttal", func(_ *synthesis.Artifact, s *debate.State, _ *manager.ResearchPlan) {
			s.Turns[1].Rebuttals[0].References[0].EventID = " "
		}},
		{"manager opportunity", func(_ *synthesis.Artifact, _ *debate.State, p *manager.ResearchPlan) {
			p.Opportunities[0].References[0].EventID = " "
		}},
		{"manager risk", func(_ *synthesis.Artifact, _ *debate.State, p *manager.ResearchPlan) {
			p.Risks[0].References[0].EventID = " "
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, s, p := ledgerFixture()
			tc.edit(a, s, p)
			got, err := Build(a, s, p)
			if got != nil || err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
				t.Fatalf("invalid reference reached ledger: %+v err=%v", got, err)
			}
		})
	}
}
