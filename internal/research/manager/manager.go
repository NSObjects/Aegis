package manager

import (
	"Aegis/internal/research/debate"
	"Aegis/internal/research/synthesis"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type ResearchManager struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

type managerInput struct {
	Artifact *synthesis.Artifact `json:"research_artifact"`
	Debate   *debate.State       `json:"debate_state"`
}

func (m *ResearchManager) BuildPlan(ctx context.Context, artifact *synthesis.Artifact, state *debate.State, opts ...model.Option) (*ResearchPlan, error) {
	if m == nil || m.Model == nil {
		return nil, errors.New("research manager model is required")
	}
	if err := synthesis.Validate(artifact); err != nil {
		return nil, fmt.Errorf("invalid research artifact: %w", err)
	}
	if err := debate.ValidateState(state, artifact); err != nil {
		return nil, fmt.Errorf("invalid debate state: %w", err)
	}
	if len(state.Turns) == 0 {
		return nil, errors.New("research manager requires a non-empty debate")
	}

	payload, err := json.Marshal(managerInput{Artifact: artifact, Debate: state})
	if err != nil {
		return nil, fmt.Errorf("encode research manager input: %w", err)
	}
	messages := []*schema.AgenticMessage{
		schema.SystemAgenticMessage(researchManagerInstruction),
		schema.UserAgenticMessage(string(payload)),
	}
	known := debateArgumentIDs(state)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil { return nil, err }
		response, err := m.Model.Generate(ctx, messages, opts...)
		if err != nil { return nil, fmt.Errorf("generate research plan: %w", err) }
		raw := strings.TrimSpace(managerAssistantText(response))
		if raw == "" { lastErr = errors.New("research manager returned empty output") } else {
			plan, decodeErr := decodeResearchPlan(raw)
			if decodeErr != nil { lastErr = decodeErr } else if validationErr := Validate(&plan, artifact, state); validationErr == nil {
				return &plan, nil
			} else {
				lastErr = fmt.Errorf("invalid research plan: %w", validationErr)
				// Model correction was exhausted. Unknown IDs are unsupported
				// provenance links, not evidence. Remove just those links, retain
				// artifact-bound claims, and disclose the loss of provenance.
				// Other schema and reference failures must remain fatal.
				if attempt == 1 && ValidateForTrading(&plan, artifact) == nil {
					corrected, removed := omitUnknownManagerLinks(plan, known)
					if removed > 0 && Validate(&corrected, artifact, state) == nil {
						return &corrected, nil
					}
				}
			}
		}
		if attempt == 0 {
			allowed, _ := json.Marshal(known)
			messages = append(messages, response, schema.UserAgenticMessage(
				fmt.Sprintf("Your previous result failed validation: %v. Return the FULL corrected ResearchPlan JSON. These are the ONLY permitted debate argument IDs: %s. Preserve the input facts. For any unknown or uncertain link, OMIT its ID rather than inventing a replacement. Do not add IDs unsupported by the debate.", lastErr, allowed),
			))
		}
	}
	return nil, lastErr
}

// debateArgumentIDs enumerates the only IDs that can be attributed to the
// validated debate. The model never supplies this allow-list.
func debateArgumentIDs(state *debate.State) []string {
	ids := make([]string, 0)
	for _, turn := range state.Turns {
		for _, argument := range turn.Arguments { ids = append(ids, argument.ID) }
	}
	return ids
}

const missingManagerArgumentProvenance = "Research Manager omitted invalid debate argument ID references; the removed links are unverified and must not be relied upon."

// This is a last-resort provenance reduction, not a substitute for validation.
// It never creates debate arguments, new references or substantive claims.
func omitUnknownManagerLinks(plan ResearchPlan, known []string) (ResearchPlan, int) {
	allowed := make(map[string]struct{},len(known))
	for _, id := range known { allowed[id] = struct{}{} }
	filter := func(ids []string) ([]string,int) {
		out := make([]string,0,len(ids))
		removed := 0
		for _,id := range ids { if _,ok:=allowed[id];ok {out=append(out,id)} else {removed++} }
		return out,removed
	}
	count := 0
	var n int
	plan.AcceptedArgumentIDs,n = filter(plan.AcceptedArgumentIDs);count+=n
	plan.UnresolvedArgumentIDs,n = filter(plan.UnresolvedArgumentIDs);count+=n
	plan.Opportunities = append([]Insight(nil),plan.Opportunities...)
	plan.Risks = append([]Insight(nil),plan.Risks...)
	for i:=range plan.Opportunities {
		plan.Opportunities[i].ArgumentIDs,n = filter(plan.Opportunities[i].ArgumentIDs)
		count+=n
	}
	for i:=range plan.Risks {
		plan.Risks[i].ArgumentIDs,n = filter(plan.Risks[i].ArgumentIDs)
		count+=n
	}
	if count>0 {
		plan.TradingConstraints=append(append([]string(nil),plan.TradingConstraints...),missingManagerArgumentProvenance)
	}
	return plan,count
}

func decodeResearchPlan(raw string) (ResearchPlan, error) {
	var plan ResearchPlan
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return ResearchPlan{}, fmt.Errorf("decode research plan JSON: %w: output=%q", err, raw)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return ResearchPlan{}, fmt.Errorf("decode research plan JSON: %w: output=%q", err, raw)
	}
	return plan, nil
}

func managerAssistantText(message *schema.AgenticMessage) string {
	if message == nil || message.Role != schema.AgenticRoleTypeAssistant {
		return ""
	}
	var b strings.Builder
	for _, block := range message.ContentBlocks {
		if block != nil && block.Type == schema.ContentBlockTypeAssistantGenText && block.AssistantGenText != nil {
			b.WriteString(block.AssistantGenText.Text)
		}
	}
	return b.String()
}

const researchManagerInstruction = `
【输出语言强制要求】所有面向人阅读的分析、结论、观点、理由、风险、约束和解释必须使用简体中文，不要整段使用英文。仅 JSON 字段名、固定枚举值、证券代码、模型/技术缩写、事件/论点/文档 ID、公司正式名称和准确的外文专有名词可保留原文。不得因翻译改变已有事实、引用 ID、数值、时间、来源或证据内容。
You are the Research Manager in Aegis.

You receive:
- one validated ResearchArtifact
- one validated completed Bull/Bear DebateState

Your job is to judge the research debate and produce a structured ResearchPlan for the downstream Trading Agent.

Hard constraints:
- Use ONLY the supplied research_artifact and debate_state.
- Do not call tools or perform new research.
- Do not invent facts, event IDs, argument IDs, prices, forecasts, or evidence.
- Do not output BUY, SELL, WATCH, AVOID, CANDIDATE, position size, or price targets.
- This is a research judgment layer, not the final trading decision layer.
- Every opportunity and risk must cite existing ResearchArtifact references.
- An insight may optionally include argument_ids to link the insight to genuinely supporting existing Bull/Bear debate arguments. These IDs must be taken from debate_state; omit argument_ids rather than guessing links when the insight cannot be reliably traced to those arguments.
- accepted_argument_ids and unresolved_argument_ids may reference only argument IDs already present in debate_state.
- An argument ID may not appear in both accepted_argument_ids and unresolved_argument_ids.
- trading_constraints are qualitative constraints for the downstream Trading Agent; they must not contain final actions, sizing, or price targets.
- Preserve uncertainty and conflicts instead of forcing false certainty.
- confidence must be between 0 and 1.
- Return one JSON object only, with no markdown.

Conclusion meanings:
- BULL_LEAN: the validated debate leaves the research case tilted bullish.
- BEAR_LEAN: the validated debate leaves the research case tilted bearish.
- BALANCED: meaningful bullish and bearish evidence remain in tension.
- INSUFFICIENT_EVIDENCE: the supplied evidence/debate is not strong enough for a directional research lean.

Schema:
{
  "symbol":"600519.SH",
  "conclusion":"BULL_LEAN|BEAR_LEAN|BALANCED|INSUFFICIENT_EVIDENCE",
  "summary":"string",
  "opportunities":[
    {"claim":"string","references":[{"domain":"TECHNICAL"}],"argument_ids":["existing_argument_id"]}
  ],
  "risks":[
    {"claim":"string","references":[{"domain":"EVENT","event_id":"existing_event_id"}],"argument_ids":["existing_argument_id"]}
  ],
  "accepted_argument_ids":["existing_argument_id"],
  "unresolved_argument_ids":["existing_argument_id"],
  "trading_constraints":["qualitative constraint"],
  "confidence":0.0
}`
