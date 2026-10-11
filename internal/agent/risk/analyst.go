package risk

import (
	"Aegis/internal/portfolio"
	riskctx "Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Analyst struct {
	Model model.BaseModel[*schema.AgenticMessage]
}

type analystInput struct {
	TradeIntent     *tradeintent.Intent `json:"trade_intent"`
	Portfolio       *portfolio.Context  `json:"portfolio_context"`
	ObservedExposure *portfolio.Exposure `json:"observed_exposure,omitempty"`
}

func (a *Analyst) Review(
	ctx context.Context,
	intent *tradeintent.Intent,
	portfolioCtx *portfolio.Context,
	opts ...model.Option,
) (*riskctx.Context, error) {
	if err := tradeintent.Validate(intent); err != nil {
		return nil, fmt.Errorf("invalid trade intent: %w", err)
	}
	if err := portfolio.Validate(portfolioCtx); err != nil {
		return nil, fmt.Errorf("invalid portfolio context: %w", err)
	}

	if portfolioCtx.State == portfolio.StateNotProvided {
		result := &riskctx.Context{
			Symbol:     intent.Symbol,
			Assessment: riskctx.AssessmentInsufficientContext,
			Summary:    "portfolio context was not provided, so portfolio-level risk cannot be assessed",
			Unresolved: []riskctx.Item{
				{
					Claim: "portfolio-level exposure and position interaction cannot be assessed from the supplied inputs",
					References: []riskctx.Reference{
						{Domain: riskctx.ReferenceTradeIntent, Symbol: intent.Symbol},
					},
				},
			},
			Confidence: 1,
		}
		if err := riskctx.Validate(result, intent, portfolioCtx); err != nil {
			return nil, fmt.Errorf("validate deterministic risk context: %w", err)
		}
		return result, nil
	}

	if a == nil || a.Model == nil {
		return nil, errors.New("risk analyst model is required")
	}

	// Exposure is calculated deterministically from an explicitly observed
	// and reconciled account valuation, never invented by the model.
	exposure, err := portfolio.ComputeExposure(portfolioCtx)
	if err != nil {
		return nil, fmt.Errorf("compute observed portfolio exposure: %w", err)
	}
	payload, err := json.Marshal(analystInput{
		TradeIntent: intent,
		Portfolio: portfolioCtx,
		ObservedExposure: exposure,
	})
	if err != nil {
		return nil, fmt.Errorf("encode risk analyst input: %w", err)
	}

	response, err := a.Model.Generate(ctx, []*schema.AgenticMessage{
		schema.SystemAgenticMessage(instruction),
		schema.UserAgenticMessage(string(payload)),
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("generate risk context: %w", err)
	}

	raw := strings.TrimSpace(assistantText(response))
	if raw == "" {
		return nil, errors.New("risk analyst returned empty output")
	}
	result, err := decodeContext(raw)
	if err != nil {
		return nil, err
	}
	if err := riskctx.Validate(&result, intent, portfolioCtx); err != nil {
		return nil, fmt.Errorf("invalid risk context: %w", err)
	}
	return &result, nil
}

func decodeContext(raw string) (riskctx.Context, error) {
	// Some JSON-mode providers add a top-level type=json_object wrapper
	// field. This is transport metadata, not part of the risk contract.
	// Only this exact field/value is tolerated; the decoder below remains
	// strict for all other unknown fields and trailing JSON.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil {
		if kind, exists := envelope["type"]; exists {
			var value string
			if err := json.Unmarshal(kind, &value); err != nil || value != "json_object" {
				return riskctx.Context{}, errors.New("decode risk context JSON: invalid type metadata")
			}
			delete(envelope, "type")
			data, err := json.Marshal(envelope)
			if err != nil { return riskctx.Context{}, fmt.Errorf("normalize risk JSON metadata: %w",err) }
			raw = string(data)
		}
	}
	var result riskctx.Context
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return riskctx.Context{}, fmt.Errorf("decode risk context JSON: %w: output=%q", err, raw)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return riskctx.Context{}, fmt.Errorf("decode risk context JSON: %w: output=%q", err, raw)
	}
	return result, nil
}

func assistantText(message *schema.AgenticMessage) string {
	if message == nil || message.Role != schema.AgenticRoleTypeAssistant {
		return ""
	}
	var b strings.Builder
	for _, block := range message.ContentBlocks {
		if block != nil &&
			block.Type == schema.ContentBlockTypeAssistantGenText &&
			block.AssistantGenText != nil {
			b.WriteString(block.AssistantGenText.Text)
		}
	}
	return b.String()
}

const instruction = `You are the General Risk Analyst in Aegis.

You receive exactly two validated inputs:
- trade_intent
- portfolio_context

Your job is to review the proposed trade intent against the explicitly observed portfolio state and produce one structured RiskContext.

Hard constraints:
- Use ONLY trade_intent and portfolio_context.
- Do not call tools or perform new market research.
- Do not invent cash, market value, cost basis, exposure, risk budget, leverage, account constraints, execution quantities, or unstated positions.
- portfolio_context.valuation is optional caller-observed CNY cash/position amounts. observed_exposure, when present, is a deterministic calculation from that observed snapshot.
- When observed_exposure is absent, concentration, weight, cash and invested value are unknown; NEVER infer them from quantity alone.
- When observed_exposure is present, cite only its observed values; weight_bps is basis points (10000 = 100%) rounded down.
- Do not infer available buying power, projected post-trade weights or limit breaches without an explicit proposed trade size and actual risk limits.
- Do not output BUY, SELL, WATCH, AVOID, CANDIDATE, orders, or position sizes.
- Do not override the Trading Agent. You are a risk review layer.
- Every finding / constraint / unresolved item must cite TRADE_INTENT and/or PORTFOLIO.
- A PORTFOLIO reference with symbol may reference only a position actually present in portfolio_context.positions.
- EMPTY means the caller explicitly provided an observed empty portfolio.
- POSITIONS means the caller explicitly provided the listed positions.
- Never infer any unprovided account facts from EMPTY or POSITIONS.
- Preserve uncertainty. If the supplied fields cannot support a specific risk claim, put it in unresolved or omit it.
- confidence must be between 0 and 1.
- Return one JSON object only, with no markdown.

Assessment meanings:
- ACCEPTABLE: no material risk concern is established from the supplied typed inputs.
- CAUTION: material risk considerations exist but the supplied inputs do not establish a high-risk condition.
- HIGH_RISK: the supplied inputs establish a material risk conflict or concentration concern.
- INSUFFICIENT_CONTEXT: the supplied typed inputs are insufficient for a meaningful risk assessment.

Schema:
{
  "symbol":"600519.SH",
  "assessment":"ACCEPTABLE|CAUTION|HIGH_RISK|INSUFFICIENT_CONTEXT",
  "summary":"string",
  "findings":[
    {
      "claim":"string",
      "references":[
        {"domain":"TRADE_INTENT","symbol":"600519.SH"},
        {"domain":"PORTFOLIO","symbol":"600519.SH"}
      ]
    }
  ],
  "constraints":[
    {
      "claim":"string",
      "references":[{"domain":"PORTFOLIO"}]
    }
  ],
  "unresolved":[
    {
      "claim":"string",
      "references":[{"domain":"TRADE_INTENT"}]
    }
  ],
  "confidence":0.0
}`
