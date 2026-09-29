package planning

import (
	"Aegis/internal/capability/event"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
)

func TestResearchPlanImplementsEinoPlan(t *testing.T) {
	var _ planexecute.Plan = (*ResearchPlan)(nil)

	plan := &ResearchPlan{
		Steps: []ResearchPlanStep{
			{
				ID:     "official",
				Need:   event.NeedOfficialDisclosure,
				Scope:  event.AcquisitionScopeDirect,
				Reason: "verify primary disclosure",
			},
		},
	}

	if got := plan.FirstStep(); got != executeResearchPlanStep {
		t.Fatalf(
			"FirstStep() = %q, want %q",
			got,
			executeResearchPlanStep,
		)
	}
}

func TestResearchPlanJSONRoundTrip(t *testing.T) {
	input := &ResearchPlan{
		Steps: []ResearchPlanStep{
			{
				ID:     "official",
				Need:   event.NeedOfficialDisclosure,
				Scope:  event.AcquisitionScopeDirect,
				Reason: "verify issuer disclosure",
			},
			{
				ID:     "news",
				Need:   event.NeedBreakingNews,
				Scope:  event.AcquisitionScopeCandidate,
				Reason: "seek independent media confirmation",
			},
		},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded ResearchPlan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(decoded.Steps) != 2 {
		t.Fatalf(
			"steps = %d, want 2",
			len(decoded.Steps),
		)
	}
	if decoded.Steps[0].Need != event.NeedOfficialDisclosure {
		t.Fatalf(
			"first need = %q",
			decoded.Steps[0].Need,
		)
	}
	if decoded.Steps[1].Scope != event.AcquisitionScopeCandidate {
		t.Fatalf(
			"second scope = %q",
			decoded.Steps[1].Scope,
		)
	}
}

func TestResearchPlanValidation(t *testing.T) {
	tests := []struct {
		name string
		plan ResearchPlan
		want string
	}{
		{
			name: "empty id",
			plan: ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						Need:  event.NeedOfficialDisclosure,
						Scope: event.AcquisitionScopeDirect,
					},
				},
			},
			want: "id is required",
		},
		{
			name: "duplicate id",
			plan: ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "same",
						Need:  event.NeedOfficialDisclosure,
						Scope: event.AcquisitionScopeDirect,
					},
					{
						ID:    "same",
						Need:  event.NeedBreakingNews,
						Scope: event.AcquisitionScopeCandidate,
					},
				},
			},
			want: "duplicated",
		},
		{
			name: "unknown need",
			plan: ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "unknown",
						Need:  event.InformationNeed("unknown"),
						Scope: event.AcquisitionScopeDirect,
					},
				},
			},
			want: "unsupported information need",
		},
		{
			name: "unknown scope",
			plan: ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "unknown",
						Need:  event.NeedOfficialDisclosure,
						Scope: event.AcquisitionScope("unknown"),
					},
				},
			},
			want: "unsupported acquisition scope",
		},
		{
			name: "official cannot be candidate",
			plan: ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "official",
						Need:  event.NeedOfficialDisclosure,
						Scope: event.AcquisitionScopeCandidate,
					},
				},
			},
			want: "must use direct scope",
		},
		{
			name: "news cannot be direct",
			plan: ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "news",
						Need:  event.NeedBreakingNews,
						Scope: event.AcquisitionScopeDirect,
					},
				},
			},
			want: "must use candidate scope",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.plan.Validate()
			if err == nil ||
				!strings.Contains(
					err.Error(),
					test.want,
				) {
				t.Fatalf(
					"error = %v, want substring %q",
					err,
					test.want,
				)
			}
		})
	}
}
