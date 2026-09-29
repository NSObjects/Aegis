package planning

import (
	"Aegis/internal/capability/event"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type scriptedModelState struct {
	responses []*schema.Message
	inputs    [][]*schema.Message
	index     int
}

type scriptedToolCallingChatModel struct {
	state *scriptedModelState
	tools []*schema.ToolInfo
}

func newScriptedToolCallingChatModel(
	responses ...*schema.Message,
) *scriptedToolCallingChatModel {
	return &scriptedToolCallingChatModel{
		state: &scriptedModelState{
			responses: responses,
		},
	}
}

func (m *scriptedToolCallingChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return nil, errors.New(
		"scripted model Generate() is not used by research plan-execute tests",
	)
}

func (m *scriptedToolCallingChatModel) Stream(
	_ context.Context,
	input []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	if m == nil || m.state == nil {
		return nil, errors.New(
			"scripted model is not initialized",
		)
	}

	copied := append(
		[]*schema.Message(nil),
		input...,
	)
	m.state.inputs = append(
		m.state.inputs,
		copied,
	)

	if m.state.index >= len(m.state.responses) {
		return nil, fmt.Errorf(
			"scripted model has no response for call %d",
			m.state.index+1,
		)
	}

	response := m.state.responses[m.state.index]
	m.state.index++

	return schema.StreamReaderFromArray(
		[]*schema.Message{response},
	), nil
}

func (m *scriptedToolCallingChatModel) WithTools(
	tools []*schema.ToolInfo,
) (model.ToolCallingChatModel, error) {
	return &scriptedToolCallingChatModel{
		state: m.state,
		tools: append(
			[]*schema.ToolInfo(nil),
			tools...,
		),
	}, nil
}

type roundTestRetriever struct {
	id       string
	docs     []event.RawDocument
	err      error
	callCount int
}

func (r *roundTestRetriever) ID() string {
	return r.id
}

func (r *roundTestRetriever) Method() event.AcquisitionMethod {
	return event.AcquisitionAPI
}

func (r *roundTestRetriever) Retrieve(
	_ context.Context,
	_ event.ResearchTask,
) ([]event.RawDocument, error) {
	r.callCount++
	documents := append(
		[]event.RawDocument(nil),
		r.docs...,
	)
	for index := range documents {
		if documents[index].AvailableAt == nil &&
			documents[index].PublishedAt != nil {
			availableAt := *documents[index].PublishedAt
			documents[index].AvailableAt = &availableAt
		}
	}
	return documents, r.err
}

type roundTestSelector struct {
	err       error
	callCount int
}

func (s *roundTestSelector) Select(
	_ context.Context,
	_ event.ResearchTask,
	documents []event.RawDocument,
) ([]event.EvidenceSelectionDecision, error) {
	s.callCount++
	if s.err != nil {
		return nil, s.err
	}

	decisions := make(
		[]event.EvidenceSelectionDecision,
		0,
		len(documents),
	)
	for _, document := range documents {
		decisions = append(
			decisions,
			event.EvidenceSelectionDecision{
				RawDocumentID: document.ID,
				Selected:      true,
				Reason:        "test relevance match",
			},
		)
	}
	return decisions, nil
}

func TestPlanExecuteAgentCompletesSatisfiedRound(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	published := asOf.Add(-time.Hour)

	initialPlan := &ResearchPlan{
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
	}

	plannerModel := newScriptedToolCallingChatModel(
		researchPlanToolCall(
			"plan-1",
			initialPlan,
		),
	)
	replannerModel := newScriptedToolCallingChatModel(
		respondToolCall(
			"respond-1",
			"research evidence is complete",
		),
	)

	official := &roundTestRetriever{
		id: "official",
		docs: []event.RawDocument{
			{
				ID:          "official-1",
				Title:       "issuer disclosure",
				URL:         "https://official.example/1",
				Publisher:   "official",
				SourceType:  event.SourceOfficial,
				IsPrimary:   true,
				PublishedAt: &published,
			},
		},
	}
	news := &roundTestRetriever{
		id: "news",
		docs: []event.RawDocument{
			{
				ID:          "news-1",
				Title:       "independent media confirmation",
				URL:         "https://media.example/1",
				Publisher:   "media",
				SourceType:  event.SourceIndustryMedia,
				PublishedAt: &published,
			},
		},
	}
	selector := &roundTestSelector{}
	resolverCalls := 0

	agent, err := NewPlanExecuteAgent(
		context.Background(),
		PlanExecuteConfig{
			PlannerModel:   plannerModel,
			ReplannerModel: replannerModel,
			Executor: ResearchRoundExecutorConfig{
				ResolveRootTask: func(
					context.Context,
					[]adk.Message,
				) (event.ResearchTask, error) {
					resolverCalls++
					return completeRootTask(
						asOf,
					), nil
				},
				Compiler: Compiler{
					Channels: ChannelBindings{
						OfficialDisclosure: official,
						BreakingNews:       news,
						Selector:           selector,
					},
				},
				Runner: event.AcquisitionPlanRunner{
					Now: func() time.Time {
						return asOf.Add(time.Minute)
					},
				},
			},
			MaxIterations: 3,
		},
	)
	if err != nil {
		t.Fatalf(
			"NewPlanExecuteAgent() error = %v",
			err,
		)
	}

	events := runPlanExecuteAgent(
		t,
		agent,
	)
	assertNoAgentErrors(t, events)

	if resolverCalls != 1 {
		t.Fatalf(
			"resolver calls = %d, want 1",
			resolverCalls,
		)
	}
	if official.callCount != 1 ||
		news.callCount != 1 {
		t.Fatalf(
			"retriever calls official=%d news=%d, want 1/1",
			official.callCount,
			news.callCount,
		)
	}
	if selector.callCount != 1 {
		t.Fatalf(
			"selector calls = %d, want 1",
			selector.callCount,
		)
	}

	replannerInput := modelInputText(
		replannerModel.state.inputs,
		0,
	)
	if !strings.Contains(
		replannerInput,
		`"status":"satisfied"`,
	) {
		t.Fatalf(
			"replanner input does not contain satisfied round result:\n%s",
			replannerInput,
		)
	}
	if !strings.Contains(
		replannerInput,
		`"document_count":2`,
	) {
		t.Fatalf(
			"replanner input does not contain cumulative document count:\n%s",
			replannerInput,
		)
	}
}

func TestPlanExecuteAgentReplansRemainingEvidenceAndAccumulatesRounds(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	published := asOf.Add(-time.Hour)

	initialPlan := &ResearchPlan{
		Steps: []ResearchPlanStep{
			{
				ID:    "official",
				Need:  event.NeedOfficialDisclosure,
				Scope: event.AcquisitionScopeDirect,
			},
		},
	}
	secondPlan := &ResearchPlan{
		Steps: []ResearchPlanStep{
			{
				ID:    "news",
				Need:  event.NeedBreakingNews,
				Scope: event.AcquisitionScopeCandidate,
			},
		},
	}

	plannerModel := newScriptedToolCallingChatModel(
		researchPlanToolCall(
			"plan-1",
			initialPlan,
		),
	)
	replannerModel := newScriptedToolCallingChatModel(
		researchPlanToolCall(
			"replan-1",
			secondPlan,
		),
		respondToolCall(
			"respond-2",
			"research evidence is complete after second round",
		),
	)

	official := &roundTestRetriever{
		id: "official",
		docs: []event.RawDocument{
			{
				ID:          "official-1",
				Title:       "issuer disclosure",
				URL:         "https://official.example/1",
				Publisher:   "official",
				SourceType:  event.SourceOfficial,
				IsPrimary:   true,
				PublishedAt: &published,
			},
		},
	}
	news := &roundTestRetriever{
		id: "news",
		docs: []event.RawDocument{
			{
				ID:          "news-1",
				Title:       "media evidence",
				URL:         "https://media.example/1",
				Publisher:   "media",
				SourceType:  event.SourceIndustryMedia,
				PublishedAt: &published,
			},
		},
	}
	selector := &roundTestSelector{}
	resolverCalls := 0
	artifacts := make([]ResearchRoundArtifact, 0, 2)

	agent, err := NewPlanExecuteAgent(
		context.Background(),
		PlanExecuteConfig{
			PlannerModel:   plannerModel,
			ReplannerModel: replannerModel,
			Executor: ResearchRoundExecutorConfig{
				ResolveRootTask: func(
					context.Context,
					[]adk.Message,
				) (event.ResearchTask, error) {
					resolverCalls++
					return completeRootTask(
						asOf,
					), nil
				},
				Compiler: Compiler{
					Channels: ChannelBindings{
						OfficialDisclosure: official,
						BreakingNews:       news,
						Selector:           selector,
					},
				},
				Runner: event.AcquisitionPlanRunner{
					Now: func() time.Time {
						return asOf.Add(time.Minute)
					},
				},
				OnRoundCompleted: func(
					_ context.Context,
					artifact ResearchRoundArtifact,
				) {
					artifacts = append(
						artifacts,
						artifact,
					)
				},
			},
			MaxIterations: 4,
		},
	)
	if err != nil {
		t.Fatalf(
			"NewPlanExecuteAgent() error = %v",
			err,
		)
	}

	events := runPlanExecuteAgent(
		t,
		agent,
	)
	assertNoAgentErrors(t, events)

	if resolverCalls != 1 {
		t.Fatalf(
			"resolver calls = %d, want 1; root task must stay stable across rounds",
			resolverCalls,
		)
	}
	if official.callCount != 1 ||
		news.callCount != 1 {
		t.Fatalf(
			"retriever calls official=%d news=%d, want 1/1",
			official.callCount,
			news.callCount,
		)
	}
	if len(replannerModel.state.inputs) != 2 {
		t.Fatalf(
			"replanner calls = %d, want 2",
			len(replannerModel.state.inputs),
		)
	}
	if len(artifacts) != 2 {
		t.Fatalf(
			"round artifacts = %d, want 2",
			len(artifacts),
		)
	}
	if artifacts[0].Round != 1 ||
		artifacts[0].Result.Status !=
			event.TaskPartial ||
		len(artifacts[0].Canonicalization.Documents) != 1 {
		t.Fatalf(
			"round 1 artifact = %+v",
			artifacts[0],
		)
	}
	if artifacts[1].Round != 2 ||
		artifacts[1].Result.Status !=
			event.TaskSatisfied ||
		len(artifacts[1].Canonicalization.Documents) != 2 ||
		len(artifacts[1].Attempts) != 2 {
		t.Fatalf(
			"round 2 artifact = %+v",
			artifacts[1],
		)
	}
	if artifacts[1].RootTask.ID !=
		"root-event-research" {
		t.Fatalf(
			"artifact root task = %+v",
			artifacts[1].RootTask,
		)
	}

	firstInput := modelInputText(
		replannerModel.state.inputs,
		0,
	)
	if !strings.Contains(
		firstInput,
		`"status":"partial"`,
	) {
		t.Fatalf(
			"first replanner input does not contain partial result:\n%s",
			firstInput,
		)
	}
	if !strings.Contains(
		firstInput,
		string(event.GapMissingRequiredSourceType),
	) {
		t.Fatalf(
			"first replanner input does not contain source gap:\n%s",
			firstInput,
		)
	}

	secondInput := modelInputText(
		replannerModel.state.inputs,
		1,
	)
	if !strings.Contains(
		secondInput,
		`"status":"satisfied"`,
	) {
		t.Fatalf(
			"second replanner input does not contain satisfied result:\n%s",
			secondInput,
		)
	}
	if !strings.Contains(
		secondInput,
		`"round":2`,
	) {
		t.Fatalf(
			"second replanner input does not contain round 2 result:\n%s",
			secondInput,
		)
	}
	if !strings.Contains(
		secondInput,
		`"document_count":2`,
	) {
		t.Fatalf(
			"second replanner input does not contain accumulated evidence:\n%s",
			secondInput,
		)
	}
}

func TestPlanExecuteAgentKeepsRetrieverFailureInResearchSemantics(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)

	plannerModel := newScriptedToolCallingChatModel(
		researchPlanToolCall(
			"plan-failure",
			&ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "official",
						Need:  event.NeedOfficialDisclosure,
						Scope: event.AcquisitionScopeDirect,
					},
				},
			},
		),
	)
	replannerModel := newScriptedToolCallingChatModel(
		respondToolCall(
			"respond-failure",
			"research remains unsatisfied",
		),
	)

	official := &roundTestRetriever{
		id:  "official",
		err: errors.New("provider unavailable"),
	}

	agent, err := NewPlanExecuteAgent(
		context.Background(),
		PlanExecuteConfig{
			PlannerModel:   plannerModel,
			ReplannerModel: replannerModel,
			Executor: ResearchRoundExecutorConfig{
				ResolveRootTask: func(
					context.Context,
					[]adk.Message,
				) (event.ResearchTask, error) {
					return event.ResearchTask{
						ID:     "root-failure",
						Symbol: "600519.SH",
						AsOf:   asOf,
						Requirements: event.ResearchRequirements{
							PITSafe:             true,
							RequiredSourceTypes: []event.SourceType{event.SourceOfficial},
							MinDocuments:        1,
						},
					}, nil
				},
				Compiler: Compiler{
					Channels: ChannelBindings{
						OfficialDisclosure: official,
					},
				},
			},
			MaxIterations: 2,
		},
	)
	if err != nil {
		t.Fatalf(
			"NewPlanExecuteAgent() error = %v",
			err,
		)
	}

	events := runPlanExecuteAgent(
		t,
		agent,
	)
	assertNoAgentErrors(t, events)

	input := modelInputText(
		replannerModel.state.inputs,
		0,
	)
	if !strings.Contains(
		input,
		`"status":"unsatisfied"`,
	) {
		t.Fatalf(
			"replanner input does not contain unsatisfied result:\n%s",
			input,
		)
	}
	if !strings.Contains(
		input,
		`"status":"failed"`,
	) {
		t.Fatalf(
			"replanner input does not preserve acquisition failure:\n%s",
			input,
		)
	}
}

func TestPlanExecuteAgentStopsOnStructuralErrors(
	t *testing.T,
) {
	asOf := time.Date(
		2026, 9, 29, 15, 0, 0, 0, time.UTC,
	)
	published := asOf.Add(-time.Hour)

	tests := []struct {
		name        string
		plan        *ResearchPlan
		resolver    RootTaskResolver
		compiler    Compiler
		wantError   string
	}{
		{
			name: "resolver failure",
			plan: &ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "official",
						Need:  event.NeedOfficialDisclosure,
						Scope: event.AcquisitionScopeDirect,
					},
				},
			},
			resolver: func(
				context.Context,
				[]adk.Message,
			) (event.ResearchTask, error) {
				return event.ResearchTask{},
					errors.New("invalid external request")
			},
			compiler: Compiler{},
			wantError: "resolve research root task",
		},
		{
			name: "compiler failure",
			plan: &ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "official",
						Need:  event.NeedOfficialDisclosure,
						Scope: event.AcquisitionScopeDirect,
					},
				},
			},
			resolver: func(
				context.Context,
				[]adk.Message,
			) (event.ResearchTask, error) {
				return event.ResearchTask{
					ID:   "root-no-symbol",
					AsOf: asOf,
				}, nil
			},
			compiler: Compiler{
				Channels: ChannelBindings{
					OfficialDisclosure: &roundTestRetriever{
						id: "official",
					},
				},
			},
			wantError: "requires root symbol",
		},
		{
			name: "selector failure",
			plan: &ResearchPlan{
				Steps: []ResearchPlanStep{
					{
						ID:    "news",
						Need:  event.NeedBreakingNews,
						Scope: event.AcquisitionScopeCandidate,
					},
				},
			},
			resolver: func(
				context.Context,
				[]adk.Message,
			) (event.ResearchTask, error) {
				return event.ResearchTask{
					ID:     "root-selector",
					Symbol: "600519.SH",
					AsOf:   asOf,
					Requirements: event.ResearchRequirements{
						PITSafe: true,
					},
				}, nil
			},
			compiler: Compiler{
				Channels: ChannelBindings{
					BreakingNews: &roundTestRetriever{
						id: "news",
						docs: []event.RawDocument{
							{
								ID:          "news-1",
								Title:       "candidate",
								URL:         "https://media.example/1",
								Publisher:   "media",
								SourceType:  event.SourceIndustryMedia,
								PublishedAt: &published,
							},
						},
					},
					Selector: &roundTestSelector{
						err: errors.New(
							"selector unavailable",
						),
					},
				},
			},
			wantError: "selector unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plannerModel :=
				newScriptedToolCallingChatModel(
					researchPlanToolCall(
						"plan-structural",
						test.plan,
					),
				)
			replannerModel :=
				newScriptedToolCallingChatModel(
					respondToolCall(
						"unexpected",
						"must not reach replanner",
					),
				)

			agent, err := NewPlanExecuteAgent(
				context.Background(),
				PlanExecuteConfig{
					PlannerModel:   plannerModel,
					ReplannerModel: replannerModel,
					Executor: ResearchRoundExecutorConfig{
						ResolveRootTask: test.resolver,
						Compiler:        test.compiler,
					},
					MaxIterations: 2,
				},
			)
			if err != nil {
				t.Fatalf(
					"NewPlanExecuteAgent() error = %v",
					err,
				)
			}

			events := runPlanExecuteAgent(
				t,
				agent,
			)
			if !hasAgentErrorContaining(
				events,
				test.wantError,
			) {
				t.Fatalf(
					"events do not contain error %q: %+v",
					test.wantError,
					events,
				)
			}
			if len(replannerModel.state.inputs) != 0 {
				t.Fatalf(
					"replanner calls = %d, want 0 after structural error",
					len(replannerModel.state.inputs),
				)
			}
		})
	}
}

func TestNewPlanExecuteAgentValidation(t *testing.T) {
	model := newScriptedToolCallingChatModel()

	_, err := NewPlanExecuteAgent(
		context.Background(),
		PlanExecuteConfig{
			ReplannerModel: model,
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"planner model is required",
		) {
		t.Fatalf(
			"error = %v, want planner-model validation",
			err,
		)
	}

	_, err = NewPlanExecuteAgent(
		context.Background(),
		PlanExecuteConfig{
			PlannerModel:   model,
			ReplannerModel: model,
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"root task resolver is required",
		) {
		t.Fatalf(
			"error = %v, want resolver validation",
			err,
		)
	}
}

func completeRootTask(
	asOf time.Time,
) event.ResearchTask {
	return event.ResearchTask{
		ID:              "root-event-research",
		Symbol:          "600519.SH",
		Objective:       "collect event evidence",
		InformationNeed: event.NeedEventEvidence,
		AsOf:            asOf,
		Requirements: event.ResearchRequirements{
			PITSafe: true,
			RequiredSourceTypes: []event.SourceType{
				event.SourceOfficial,
				event.SourceIndustryMedia,
			},
			MinDocuments:          2,
			MinIndependentSources: 2,
			RequirePrimarySource:  true,
		},
	}
}

func researchPlanToolCall(
	id string,
	plan *ResearchPlan,
) *schema.Message {
	data, err := json.Marshal(plan)
	if err != nil {
		panic(err)
	}

	return schema.AssistantMessage(
		"",
		[]schema.ToolCall{
			{
				ID:   id,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      ResearchPlanToolInfo.Name,
					Arguments: string(data),
				},
			},
		},
	)
}

func respondToolCall(
	id string,
	response string,
) *schema.Message {
	data, err := json.Marshal(
		map[string]string{
			"response": response,
		},
	)
	if err != nil {
		panic(err)
	}

	return schema.AssistantMessage(
		"",
		[]schema.ToolCall{
			{
				ID:   id,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "respond",
					Arguments: string(data),
				},
			},
		},
	)
}

func runPlanExecuteAgent(
	t *testing.T,
	agent adk.ResumableAgent,
) []*adk.AgentEvent {
	t.Helper()

	ctx := context.Background()
	runner := adk.NewRunner(
		ctx,
		adk.RunnerConfig{
			Agent: agent,
		},
	)

	iter := runner.Run(
		ctx,
		[]adk.Message{
			schema.UserMessage(
				"research 600519.SH event evidence",
			),
		},
	)

	events := make(
		[]*adk.AgentEvent,
		0,
	)
	for {
		e, ok := iter.Next()
		if !ok {
			break
		}
		if e != nil {
			events = append(
				events,
				e,
			)
		}
	}

	return events
}

func assertNoAgentErrors(
	t *testing.T,
	events []*adk.AgentEvent,
) {
	t.Helper()

	for _, event := range events {
		if event.Err != nil {
			t.Fatalf(
				"unexpected agent error: %v",
				event.Err,
			)
		}
	}
}

func hasAgentErrorContaining(
	events []*adk.AgentEvent,
	substring string,
) bool {
	for _, event := range events {
		if event.Err != nil &&
			strings.Contains(
				event.Err.Error(),
				substring,
			) {
			return true
		}
	}
	return false
}

func modelInputText(
	inputs [][]*schema.Message,
	index int,
) string {
	if index < 0 || index >= len(inputs) {
		return ""
	}

	var builder strings.Builder
	for _, message := range inputs[index] {
		if message == nil {
			continue
		}
		builder.WriteString(message.Content)
		builder.WriteString("\n")
	}
	return builder.String()
}
