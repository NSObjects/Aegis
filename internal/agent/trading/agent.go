package trading

import (
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/decision"
	"Aegis/internal/research/synthesis"
	"Aegis/internal/research/manager"
	"Aegis/internal/researchlang"
	"Aegis/internal/tradeintent"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Config struct {
	Model model.BaseModel[*schema.AgenticMessage]

	MaxIterations int
}

type Input struct {
	Symbol           string
	MarketContext    *market.Context
	TechnicalContext *technical.Context
}

type Agent struct {
	runner         *adk.TypedRunner[*schema.AgenticMessage]
	artifactRunner *adk.TypedRunner[*schema.AgenticMessage]
	planRunner     *adk.TypedRunner[*schema.AgenticMessage]
}

func New(
	ctx context.Context,
	cfg Config,
) (*Agent, error) {
	if cfg.Model == nil {
		return nil, errors.New(
			"trading agent model is required",
		)
	}

	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 20
	}

	legacyRunner, err := newTradingRunner(
		ctx,
		cfg.Model,
		maxIterations,
		"trading_agent_legacy",
		"Consumes legacy market and technical contexts and produces a structured decision.",
		instruction,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create legacy trading runner: %w",
			err,
		)
	}

	artifactRunner, err := newTradingRunner(
		ctx,
		cfg.Model,
		maxIterations,
		"trading_agent",
		"Consumes a validated ResearchArtifact and produces a structured decision.",
		artifactInstruction,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create artifact trading runner: %w",
			err,
		)
	}

	planRunner, err := newTradingRunner(
		ctx,
		cfg.Model,
		maxIterations,
		"trading_agent_research_plan",
		"Consumes a validated ResearchArtifact plus ResearchPlan and produces a structured decision.",
		researchPlanInstruction,
	)
	if err != nil {
		return nil, fmt.Errorf("create research-plan trading runner: %w", err)
	}

	return &Agent{
		runner:         legacyRunner,
		artifactRunner: artifactRunner,
		planRunner:     planRunner,
	}, nil
}

func newTradingRunner(
	ctx context.Context,
	m model.BaseModel[*schema.AgenticMessage],
	maxIterations int,
	name string,
	description string,
	instructionText string,
) (*adk.TypedRunner[*schema.AgenticMessage], error) {
	a, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](
		ctx,
		&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:          name,
			Description:   description,
			Instruction:   instructionText,
			Model:         m,
			MaxIterations: maxIterations,
		},
	)
	if err != nil {
		return nil, err
	}

	return adk.NewTypedRunner(
		adk.TypedRunnerConfig[*schema.AgenticMessage]{
			Agent:           a,
			EnableStreaming: false,
		},
	), nil
}

// Analyze is the legacy MarketContext + TechnicalContext trading path.
// The main research graph uses AnalyzeArtifact.
func (a *Agent) Analyze(
	ctx context.Context,
	input Input,
	opts ...adk.AgentRunOption,
) (
	*decision.Decision,
	error,
) {
	if a == nil || a.runner == nil {
		return nil, errors.New(
			"trading agent is not initialized",
		)
	}

	symbol := strings.TrimSpace(input.Symbol)
	if symbol == "" {
		return nil, errors.New(
			"trading symbol is required",
		)
	}

	if err := market.Validate(input.MarketContext); err != nil {
		return nil, fmt.Errorf(
			"invalid market context: %w",
			err,
		)
	}

	if err := technical.Validate(input.TechnicalContext); err != nil {
		return nil, fmt.Errorf(
			"invalid technical context: %w",
			err,
		)
	}

	if input.TechnicalContext.Symbol != symbol {
		return nil, fmt.Errorf(
			"technical context symbol %q does not match trading symbol %q",
			input.TechnicalContext.Symbol,
			symbol,
		)
	}

	if input.TechnicalContext.RequestedAsOf != input.MarketContext.AsOf {
		return nil, fmt.Errorf(
			"technical requested_as_of %q does not match market as_of %q",
			input.TechnicalContext.RequestedAsOf,
			input.MarketContext.AsOf,
		)
	}

	marketJSON, err := json.Marshal(input.MarketContext)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal market context: %w",
			err,
		)
	}

	technicalJSON, err := json.Marshal(input.TechnicalContext)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal technical context: %w",
			err,
		)
	}

	query := fmt.Sprintf(
		"研究标的：%s\n\n上游 market_context：\n%s\n\n上游 technical_context：\n%s\n\n请仅基于以上结构化研究上下文综合判断，并输出最终 Decision。",
		symbol,
		string(marketJSON),
		string(technicalJSON),
	)

	return runDecision(
		ctx,
		a.runner,
		symbol,
		query,
		opts...,
	)
}

func (a *Agent) AnalyzeArtifact(
	ctx context.Context,
	artifact *synthesis.Artifact,
	opts ...adk.AgentRunOption,
) (
	*decision.Decision,
	error,
) {
	if a == nil || a.artifactRunner == nil {
		return nil, errors.New(
			"trading artifact runner is not initialized",
		)
	}
	if err := synthesis.Validate(artifact); err != nil {
		return nil, fmt.Errorf(
			"invalid research artifact: %w",
			err,
		)
	}

	symbol := strings.TrimSpace(
		artifact.Symbol,
	)
	if symbol == "" {
		return nil, errors.New(
			"research artifact symbol is required",
		)
	}

	data, err := json.Marshal(artifact)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal research artifact: %w",
			err,
		)
	}

	query := fmt.Sprintf(
		"研究标的：%s\n\n上游 research_artifact：\n%s\n\n请仅基于该结构化 ResearchArtifact 形成最终 Decision。",
		symbol,
		string(data),
	)

	return runDecision(
		ctx,
		a.artifactRunner,
		symbol,
		query,
		opts...,
	)
}


func (a *Agent) AnalyzeResearchPlan(
	ctx context.Context,
	artifact *synthesis.Artifact,
	plan *manager.ResearchPlan,
	opts ...adk.AgentRunOption,
) (*decision.Decision, error) {
	if a == nil || a.planRunner == nil {
		return nil, errors.New("trading research-plan runner is not initialized")
	}
	if err := manager.ValidateForTrading(plan, artifact); err != nil {
		return nil, fmt.Errorf("invalid research plan: %w", err)
	}

	artifactJSON, err := json.Marshal(artifact)
	if err != nil {
		return nil, fmt.Errorf("marshal research artifact: %w", err)
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("marshal research plan: %w", err)
	}

	symbol := strings.TrimSpace(artifact.Symbol)
	query := fmt.Sprintf(
		"研究标的：%s\n\n上游 research_artifact：\n%s\n\n上游 research_plan：\n%s\n\n请同时消费已验证的 ResearchArtifact 与 ResearchPlan，独立形成最终 Decision。",
		symbol,
		string(artifactJSON),
		string(planJSON),
	)
	return runDecision(ctx, a.planRunner, symbol, query, opts...)
}


func (a *Agent) AnalyzeResearchPlanIntent(
	ctx context.Context,
	artifact *synthesis.Artifact,
	plan *manager.ResearchPlan,
	opts ...adk.AgentRunOption,
) (*tradeintent.Intent, error) {
	result, err := a.AnalyzeResearchPlan(ctx, artifact, plan, opts...)
	if err != nil {
		return nil, err
	}
	intent, err := tradeintent.FromDecision(result)
	if err != nil {
		return nil, fmt.Errorf("convert trading decision to trade intent: %w", err)
	}
	return intent, nil
}

const maxTradingDecisionJSONAttempts = 2

// runDecision only retries JSON *syntax* failures, not semantic or provenance
// validation errors. A fresh model attempt must still pass Decision.Validate.
func runDecision(
	ctx context.Context,
	runner *adk.TypedRunner[*schema.AgenticMessage],
	symbol string,
	query string,
	opts ...adk.AgentRunOption,
) (*decision.Decision, error) {
	for attempt := 1; attempt <= maxTradingDecisionJSONAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		result, err := runDecisionOnce(ctx, runner, symbol, query, opts...)
		if err == nil {
			return result, nil
		}

		var syntaxErr *json.SyntaxError
        languageErr:=errors.Is(err,researchlang.ErrLanguage)
        if (!errors.As(err,&syntaxErr) && !languageErr) || attempt==maxTradingDecisionJSONAttempts {
            return nil,fmt.Errorf("trading decision attempt %d/%d: %w",attempt,maxTradingDecisionJSONAttempts,err)
        }
        // Regenerate using the identical evidence. Never patch JSON facts or citations.
        if languageErr {
            query+="\n\n"+researchlang.Correction
        } else {
            query+="\n\n上一轮输出包含 JSON 语法错误。请基于同一份上游研究材料重新生成完整的合法 JSON object。确保 evidence 和 risk 是有效字符串数组，不新增事实。"
        }
	}
	return nil, errors.New("trading decision retry exhausted")
}

func runDecisionOnce(
	ctx context.Context,
	runner *adk.TypedRunner[*schema.AgenticMessage],
	symbol string,
	query string,
	opts ...adk.AgentRunOption,
) (*decision.Decision, error) {
	iter := runner.Query(ctx, query, opts...)
	var output string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return nil, fmt.Errorf("trading agent run failed: %w", event.Err)
		}
		if event.Output == nil ||
			event.Output.MessageOutput == nil ||
			event.Output.MessageOutput.Message == nil {
			continue
		}
		text := strings.TrimSpace(assistantText(event.Output.MessageOutput.Message))
		if text != "" {
			output = text
		}
	}
	if output == "" {
		return nil, errors.New("trading agent returned empty output")
	}
	result, err := parseDecision(output)
	if err != nil {
		return nil, err
	}
	if result.Symbol != symbol {
		return nil, fmt.Errorf(
			"decision symbol %q does not match input symbol %q",
			result.Symbol, symbol,
		)
	}
	fields:=[]researchlang.Field{{Name:"decision.market_view",Text:result.MarketView}}
    for _,line:=range result.Evidence{fields=append(fields,researchlang.Field{Name:"decision.evidence",Text:line})}
    for _,line:=range result.Risk{fields=append(fields,researchlang.Field{Name:"decision.risk",Text:line})}
    if err:=researchlang.Check(ctx,fields...);err!=nil{return nil,err}
    return result,nil
}

func parseDecision(
	raw string,
) (
	*decision.Decision,
	error,
) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New(
			"trading decision output is empty",
		)
	}

	var result decision.Decision

	if err := json.Unmarshal(
		[]byte(raw),
		&result,
	); err != nil {
		return nil, fmt.Errorf(
			"invalid trading decision JSON: %w: output=%q",
			err,
			raw,
		)
	}

	if err := decision.Validate(&result); err != nil {
		return nil, fmt.Errorf(
			"invalid trading decision: %w",
			err,
		)
	}

	return &result, nil
}

func assistantText(
	msg *schema.AgenticMessage,
) string {
	if msg == nil ||
		msg.Role != schema.AgenticRoleTypeAssistant {
		return ""
	}

	for _, block := range msg.ContentBlocks {
		if block != nil &&
			block.Type == schema.ContentBlockTypeFunctionToolCall &&
			block.FunctionToolCall != nil {
			return ""
		}
	}

	var b strings.Builder

	for _, block := range msg.ContentBlocks {
		if block == nil ||
			block.Type != schema.ContentBlockTypeAssistantGenText ||
			block.AssistantGenText == nil {
			continue
		}

		b.WriteString(
			block.AssistantGenText.Text,
		)
	}

	return b.String()
}
