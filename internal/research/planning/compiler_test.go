package planning

import (
	"Aegis/internal/capability/event"
	"context"
	"strings"
	"testing"
)

type compilerRetriever struct {
	id     string
	method event.AcquisitionMethod
}

func (r compilerRetriever) ID() string {
	return r.id
}

func (r compilerRetriever) Method() event.AcquisitionMethod {
	return r.method
}

func (r compilerRetriever) Retrieve(
	context.Context,
	event.ResearchTask,
) ([]event.RawDocument, error) {
	return nil, nil
}

type compilerSelector struct{}

func (compilerSelector) Select(
	context.Context,
	event.ResearchTask,
	[]event.RawDocument,
) ([]event.EvidenceSelectionDecision, error) {
	return nil, nil
}

func TestCompilerBuildsSingleProviderAcquisitionPlan(
	t *testing.T,
) {
	official := compilerRetriever{
		id:     "cninfo_announcements",
		method: event.AcquisitionAPI,
	}
	news := compilerRetriever{
		id:     "cls_news_telegraph",
		method: event.AcquisitionAPI,
	}
	selector := compilerSelector{}

	compiled, err := (Compiler{
		Channels: ChannelBindings{
			OfficialDisclosure: []event.Retriever{
				official,
			},
			BreakingNews: []event.Retriever{
				news,
			},
			Selector: selector,
		},
	}).Compile(
		event.ResearchTask{
			ID:     "root-task",
			Symbol: "600519.SH",
		},
		&ResearchPlan{
			Steps: []ResearchPlanStep{
				{
					ID:    "official",
					Need:  event.NeedOfficialDisclosure,
					Scope: event.AcquisitionScopeDirect,
				},
				{
					ID:    "news",
					Need:  event.NeedBreakingNews,
					Scope: event.AcquisitionScopeCandidate,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if len(compiled.Steps) != 2 {
		t.Fatalf(
			"compiled steps = %d, want 2",
			len(compiled.Steps),
		)
	}

	officialStep := compiled.Steps[0]
	if officialStep.ID != "official" {
		t.Fatalf(
			"single-provider official step id = %q",
			officialStep.ID,
		)
	}
	if officialStep.Scope != event.AcquisitionScopeDirect {
		t.Fatalf(
			"official scope = %q",
			officialStep.Scope,
		)
	}
	if officialStep.Task.Symbol != "600519.SH" {
		t.Fatalf(
			"official symbol = %q, want 600519.SH",
			officialStep.Task.Symbol,
		)
	}
	if officialStep.Task.InformationNeed !=
		event.NeedOfficialDisclosure {
		t.Fatalf(
			"official need = %q",
			officialStep.Task.InformationNeed,
		)
	}
	if officialStep.Retriever.ID() != official.ID() {
		t.Fatalf(
			"official retriever = %q",
			officialStep.Retriever.ID(),
		)
	}

	newsStep := compiled.Steps[1]
	if newsStep.ID != "news" {
		t.Fatalf(
			"single-provider news step id = %q",
			newsStep.ID,
		)
	}
	if newsStep.Scope != event.AcquisitionScopeCandidate {
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
	if newsStep.Retriever.ID() != news.ID() {
		t.Fatalf(
			"news retriever = %q",
			newsStep.Retriever.ID(),
		)
	}
	if compiled.Selector == nil {
		t.Fatal("selector binding should be preserved")
	}
}

func TestCompilerFansOutNeedAcrossProviders(
	t *testing.T,
) {
	compiled, err := (Compiler{
		Channels: ChannelBindings{
			BreakingNews: []event.Retriever{
				compilerRetriever{
					id:     "cls_news",
					method: event.AcquisitionAPI,
				},
				compilerRetriever{
					id:     "web_news",
					method: event.AcquisitionWebSearch,
				},
			},
			Selector: compilerSelector{},
		},
	}).Compile(
		event.ResearchTask{
			ID:     "root-task",
			Symbol: "600519.SH",
		},
		&ResearchPlan{
			Steps: []ResearchPlanStep{
				{
					ID:    "news",
					Need:  event.NeedBreakingNews,
					Scope: event.AcquisitionScopeCandidate,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compiled.Steps) != 2 {
		t.Fatalf(
			"compiled steps = %d, want 2",
			len(compiled.Steps),
		)
	}

	wantIDs := []string{
		"news@cls_news",
		"news@web_news",
	}
	wantMethods := []event.AcquisitionMethod{
		event.AcquisitionAPI,
		event.AcquisitionWebSearch,
	}
	for index, step := range compiled.Steps {
		if step.ID != wantIDs[index] {
			t.Fatalf(
				"step %d id = %q, want %q",
				index,
				step.ID,
				wantIDs[index],
			)
		}
		if step.Task.Symbol != "" {
			t.Fatalf(
				"step %d symbol = %q, want empty",
				index,
				step.Task.Symbol,
			)
		}
		if step.Retriever.Method() != wantMethods[index] {
			t.Fatalf(
				"step %d method = %q, want %q",
				index,
				step.Retriever.Method(),
				wantMethods[index],
			)
		}
	}
}

func TestCompilerOfficialFanOutPreservesRootSymbol(
	t *testing.T,
) {
	compiled, err := (Compiler{
		Channels: ChannelBindings{
			OfficialDisclosure: []event.Retriever{
				compilerRetriever{
					id:     "cninfo",
					method: event.AcquisitionAPI,
				},
				compilerRetriever{
					id:     "exchange",
					method: event.AcquisitionCrawler,
				},
			},
		},
	}).Compile(
		event.ResearchTask{
			ID:     "root-task",
			Symbol: "600519.SH",
		},
		&ResearchPlan{
			Steps: []ResearchPlanStep{
				{
					ID:    "official",
					Need:  event.NeedOfficialDisclosure,
					Scope: event.AcquisitionScopeDirect,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compiled.Steps) != 2 {
		t.Fatalf(
			"compiled steps = %d, want 2",
			len(compiled.Steps),
		)
	}
	for _, step := range compiled.Steps {
		if step.Task.Symbol != "600519.SH" {
			t.Fatalf(
				"official step %q symbol = %q",
				step.ID,
				step.Task.Symbol,
			)
		}
	}
}

func TestCompilerRejectsMissingBindings(t *testing.T) {
	root := event.ResearchTask{
		ID:     "root-task",
		Symbol: "600519.SH",
	}

	tests := []struct {
		name     string
		compiler Compiler
		plan     ResearchPlan
		want     string
	}{
		{
			name: "missing official channel",
			compiler: Compiler{
				Channels: ChannelBindings{
					BreakingNews: []event.Retriever{
						compilerRetriever{
							id:     "news",
							method: event.AcquisitionAPI,
						},
					},
				},
			},
			plan: ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "official",
						Need:  event.NeedOfficialDisclosure,
						Scope: event.AcquisitionScopeDirect,
					},
				},
			},
			want: "official disclosure channel requires at least one retriever binding",
		},
		{
			name: "missing news channel",
			compiler: Compiler{
				Channels: ChannelBindings{
					OfficialDisclosure: []event.Retriever{
						compilerRetriever{
							id:     "official",
							method: event.AcquisitionAPI,
						},
					},
				},
			},
			plan: ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "news",
						Need:  event.NeedBreakingNews,
						Scope: event.AcquisitionScopeCandidate,
					},
				},
			},
			want: "breaking news channel requires at least one retriever binding",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.compiler.Compile(
				root,
				&test.plan,
			)
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

func TestCompilerRejectsInvalidProviderBindings(
	t *testing.T,
) {
	tests := []struct {
		name       string
		retrievers []event.Retriever
		want       string
	}{
		{
			name: "nil retriever",
			retrievers: []event.Retriever{
				nil,
			},
			want: "retriever 0 is nil",
		},
		{
			name: "empty retriever id",
			retrievers: []event.Retriever{
				compilerRetriever{
					method: event.AcquisitionAPI,
				},
			},
			want: "id is required",
		},
		{
			name: "missing method",
			retrievers: []event.Retriever{
				compilerRetriever{
					id: "provider",
				},
			},
			want: "acquisition method is required",
		},
		{
			name: "duplicate retriever id",
			retrievers: []event.Retriever{
				compilerRetriever{
					id:     "provider",
					method: event.AcquisitionAPI,
				},
				compilerRetriever{
					id:     "provider",
					method: event.AcquisitionWebSearch,
				},
			},
			want: "retriever id "provider" is duplicated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (Compiler{
				Channels: ChannelBindings{
					BreakingNews: test.retrievers,
					Selector:     compilerSelector{},
				},
			}).Compile(
				event.ResearchTask{
					ID:     "root-task",
					Symbol: "600519.SH",
				},
				&ResearchPlan{
					Steps: []ResearchPlanStep{
						{
							ID:    "news",
							Need:  event.NeedBreakingNews,
							Scope: event.AcquisitionScopeCandidate,
						},
					},
				},
			)
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

func TestCompilerRequiresSelectorForCandidatePlan(
	t *testing.T,
) {
	_, err := (Compiler{
		Channels: ChannelBindings{
			BreakingNews: []event.Retriever{
				compilerRetriever{
					id:     "news",
					method: event.AcquisitionAPI,
				},
			},
		},
	}).Compile(
		event.ResearchTask{
			ID:     "root-task",
			Symbol: "600519.SH",
		},
		&ResearchPlan{
			Steps: []ResearchPlanStep{
				{
					ID:    "news",
					Need:  event.NeedBreakingNews,
					Scope: event.AcquisitionScopeCandidate,
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"requires selector binding",
		) {
		t.Fatalf(
			"error = %v, want selector-binding validation",
			err,
		)
	}
}

func TestCompilerOfficialDisclosureRequiresRootSymbol(
	t *testing.T,
) {
	_, err := (Compiler{
		Channels: ChannelBindings{
			OfficialDisclosure: []event.Retriever{
				compilerRetriever{
					id:     "official",
					method: event.AcquisitionAPI,
				},
			},
		},
	}).Compile(
		event.ResearchTask{
			ID: "root-task",
		},
		&ResearchPlan{
			Steps: []ResearchPlanStep{
				{
					ID:    "official",
					Need:  event.NeedOfficialDisclosure,
					Scope: event.AcquisitionScopeDirect,
				},
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"requires root symbol",
		) {
		t.Fatalf(
			"error = %v, want root-symbol validation",
			err,
		)
	}
}

func TestCompilerRejectsNilPlan(t *testing.T) {
	_, err := (Compiler{}).Compile(
		event.ResearchTask{
			ID: "root-task",
		},
		nil,
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"research plan is required",
		) {
		t.Fatalf(
			"error = %v, want research plan required",
			err,
		)
	}
}

func TestCompilerCoalescesRepeatedNeedBeforeProviderFanOut(
	t *testing.T,
) {
	compiled, err := (Compiler{
		Channels: ChannelBindings{
			BreakingNews: []event.Retriever{
				compilerRetriever{
					id:     "snapshot_news",
					method: event.AcquisitionSnapshotStore,
				},
				compilerRetriever{
					id:     "web_news",
					method: event.AcquisitionWebSearch,
				},
			},
			Selector: compilerSelector{},
		},
	}).Compile(
		event.ResearchTask{
			ID:     "root-task",
			Symbol: "600519.SH",
		},
		&ResearchPlan{
			Steps: []ResearchPlanStep{
				{
					ID:     "company-news",
					Need:   event.NeedBreakingNews,
					Scope:  event.AcquisitionScopeCandidate,
					Reason: "company event confirmation",
				},
				{
					ID:     "industry-news",
					Need:   event.NeedBreakingNews,
					Scope:  event.AcquisitionScopeCandidate,
					Reason: "independent media confirmation",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"Compile() error = %v",
			err,
		)
	}
	if len(compiled.Steps) != 2 {
		t.Fatalf(
			"compiled steps = %d, want two providers from first logical need",
			len(compiled.Steps),
		)
	}
	if compiled.Steps[0].ID !=
		"company-news@snapshot_news" ||
		compiled.Steps[1].ID !=
			"company-news@web_news" {
		t.Fatalf(
			"compiled step ids = %q, %q",
			compiled.Steps[0].ID,
			compiled.Steps[1].ID,
		)
	}
}
