package event

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResearchPlanJSONRoundTrip(t *testing.T) {
	plan := ResearchPlan{
		Steps: []ResearchPlanStep{
			{
				ID:              "official",
				InformationNeed: NeedOfficialDisclosure,
				SourceType:      SourceOfficial,
				Scope:           AcquisitionScopeDirect,
				Reason:          "verify issuer disclosure",
			},
			{
				ID:              "media",
				InformationNeed: NeedBreakingNews,
				SourceType:      SourceIndustryMedia,
				Scope:           AcquisitionScopeCandidate,
				Reason:          "independent media confirmation",
			},
		},
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded ResearchPlan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(decoded.Steps) != 2 {
		t.Fatalf(
			"steps = %d, want 2",
			len(decoded.Steps),
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
			name: "empty",
			plan: ResearchPlan{},
			want: "at least one step",
		},
		{
			name: "empty id",
			plan: ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						InformationNeed: NeedBreakingNews,
						SourceType:      SourceIndustryMedia,
						Scope:           AcquisitionScopeCandidate,
						Reason:          "test",
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
						ID:              "same",
						InformationNeed: NeedOfficialDisclosure,
						SourceType:      SourceOfficial,
						Scope:           AcquisitionScopeDirect,
						Reason:          "first",
					},
					{
						ID:              "same",
						InformationNeed: NeedBreakingNews,
						SourceType:      SourceIndustryMedia,
						Scope:           AcquisitionScopeCandidate,
						Reason:          "second",
					},
				},
			},
			want: "duplicated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.plan.Validate()
			if err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf(
					"error = %v, want substring %q",
					err,
					test.want,
				)
			}
		})
	}
}

func TestResearchPlanCompiler(t *testing.T) {
	root := ResearchTask{
		ID:     "task-plan",
		Symbol: "600519.SH",
		Requirements: ResearchRequirements{
			PITSafe: true,
		},
	}

	official := CninfoAnnouncementRetriever{}
	news := ClsNewsRetriever{}
	selector := fakeEvidenceSelector{
		selectFn: nil,
	}

	compiler := ResearchPlanCompiler{
		OfficialDisclosureRetriever: official,
		BreakingNewsRetriever:       news,
		Selector:                    selector,
	}

	plan, err := compiler.Compile(
		root,
		ResearchPlan{
			Steps: []ResearchPlanStep{
				{
					ID:              "official",
					InformationNeed: NeedOfficialDisclosure,
					SourceType:      SourceOfficial,
					Scope:           AcquisitionScopeDirect,
					Reason:          "verify primary disclosure",
				},
				{
					ID:              "media",
					InformationNeed: NeedBreakingNews,
					SourceType:      SourceIndustryMedia,
					Scope:           AcquisitionScopeCandidate,
					Reason:          "seek independent confirmation",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf(
			"steps = %d, want 2",
			len(plan.Steps),
		)
	}

	officialStep := plan.Steps[0]
	if officialStep.Scope != AcquisitionScopeDirect {
		t.Fatalf(
			"official scope = %q",
			officialStep.Scope,
		)
	}
	if officialStep.Task.Symbol != root.Symbol {
		t.Fatalf(
			"official symbol = %q, want %q",
			officialStep.Task.Symbol,
			root.Symbol,
		)
	}
	if officialStep.Task.InformationNeed != NeedOfficialDisclosure {
		t.Fatalf(
			"official need = %q",
			officialStep.Task.InformationNeed,
		)
	}
	if _, ok := officialStep.Retriever.(CninfoAnnouncementRetriever); !ok {
		t.Fatalf(
			"official retriever type = %T",
			officialStep.Retriever,
		)
	}

	newsStep := plan.Steps[1]
	if newsStep.Scope != AcquisitionScopeCandidate {
		t.Fatalf(
			"news scope = %q",
			newsStep.Scope,
		)
	}
	if newsStep.Task.Symbol != "" {
		t.Fatalf(
			"news symbol = %q, want empty",
			newsStep.Task.Symbol,
		)
	}
	if newsStep.Task.InformationNeed != NeedBreakingNews {
		t.Fatalf(
			"news need = %q",
			newsStep.Task.InformationNeed,
		)
	}
	if _, ok := newsStep.Retriever.(ClsNewsRetriever); !ok {
		t.Fatalf(
			"news retriever type = %T",
			newsStep.Retriever,
		)
	}
	if plan.Selector == nil {
		t.Fatal("candidate plan must bind selector")
	}
}

func TestResearchPlanCompilerRejectsUnsupportedCombination(
	t *testing.T,
) {
	_, err := (ResearchPlanCompiler{
		BreakingNewsRetriever: ClsNewsRetriever{},
		Selector: fakeEvidenceSelector{
			selectFn: nil,
		},
	}).Compile(
		ResearchTask{
			ID:     "task-invalid-plan",
			Symbol: "600519.SH",
		},
		ResearchPlan{
			Steps: []ResearchPlanStep{
				{
					ID:              "bad",
					InformationNeed: NeedBreakingNews,
					SourceType:      SourceOfficial,
					Scope:           AcquisitionScopeDirect,
					Reason:          "invalid combination",
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"unsupported combination",
		) {
		t.Fatalf(
			"error = %v, want unsupported combination",
			err,
		)
	}
}

func TestResearchPlanCompilerRequiresSelectorForCandidate(
	t *testing.T,
) {
	_, err := (ResearchPlanCompiler{
		BreakingNewsRetriever: ClsNewsRetriever{},
	}).Compile(
		ResearchTask{
			ID: "task-candidate",
		},
		ResearchPlan{
			Steps: []ResearchPlanStep{
				{
					ID:              "media",
					InformationNeed: NeedBreakingNews,
					SourceType:      SourceIndustryMedia,
					Scope:           AcquisitionScopeCandidate,
					Reason:          "candidate media",
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"requires an evidence selector",
		) {
		t.Fatalf(
			"error = %v, want selector requirement",
			err,
		)
	}
}

func TestValidateReplanDecision(t *testing.T) {
	validPlan := &ResearchPlan{
		Steps: []ResearchPlanStep{
			{
				ID:              "media",
				InformationNeed: NeedBreakingNews,
				SourceType:      SourceIndustryMedia,
				Scope:           AcquisitionScopeCandidate,
				Reason:          "fill media gap",
			},
		},
	}

	tests := []struct {
		name     string
		result   ResearchTaskResult
		decision ReplanDecision
		wantErr  bool
	}{
		{
			name: "satisfied complete",
			result: ResearchTaskResult{
				Status: TaskSatisfied,
			},
			decision: ReplanDecision{
				Action: ReplanComplete,
				Reason: "requirements satisfied",
			},
		},
		{
			name: "partial cannot complete",
			result: ResearchTaskResult{
				Status: TaskPartial,
			},
			decision: ReplanDecision{
				Action: ReplanComplete,
				Reason: "looks good",
			},
			wantErr: true,
		},
		{
			name: "unsatisfied continue",
			result: ResearchTaskResult{
				Status: TaskUnsatisfied,
			},
			decision: ReplanDecision{
				Action: ReplanContinue,
				Reason: "fill missing evidence",
				Plan:   validPlan,
			},
		},
		{
			name: "continue needs plan",
			result: ResearchTaskResult{
				Status: TaskPartial,
			},
			decision: ReplanDecision{
				Action: ReplanContinue,
				Reason: "continue",
			},
			wantErr: true,
		},
		{
			name: "satisfied cannot continue",
			result: ResearchTaskResult{
				Status: TaskSatisfied,
			},
			decision: ReplanDecision{
				Action: ReplanContinue,
				Reason: "more",
				Plan:   validPlan,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateReplanDecision(
				test.result,
				test.decision,
			)
			if test.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf(
					"unexpected error: %v",
					err,
				)
			}
		})
	}
}
