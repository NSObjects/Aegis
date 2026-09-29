package planning

import (
	"Aegis/internal/capability/event"
	"context"
	"errors"
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

func TestCompilerBuildsAcquisitionPlan(t *testing.T) {
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
			OfficialDisclosure: official,
			BreakingNews:       news,
			Selector:           selector,
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
	if officialStep.Task.InformationNeed != event.NeedOfficialDisclosure {
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
	if newsStep.Task.InformationNeed != event.NeedBreakingNews {
		t.Fatalf(
			"news need = %q",
			newsStep.Task.InformationNeed,
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
					BreakingNews: compilerRetriever{
						id:     "news",
						method: event.AcquisitionAPI,
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
			want: "official disclosure channel binding",
		},
		{
			name: "missing news channel",
			compiler: Compiler{
				Channels: ChannelBindings{
					OfficialDisclosure: compilerRetriever{
						id:     "official",
						method: event.AcquisitionAPI,
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
			want: "breaking news channel binding",
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

func TestCompilerOfficialDisclosureRequiresRootSymbol(
	t *testing.T,
) {
	_, err := (Compiler{
		Channels: ChannelBindings{
			OfficialDisclosure: compilerRetriever{
				id:     "official",
				method: event.AcquisitionAPI,
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
		!errors.Is(
			err,
			errors.New("research plan is required"),
		) && !strings.Contains(
			err.Error(),
			"research plan is required",
		) {
		t.Fatalf(
			"error = %v, want research plan required",
			err,
		)
	}
}
