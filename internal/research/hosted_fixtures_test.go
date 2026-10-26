package research

import (
	"Aegis/internal/agent/market"
	"context"

	"github.com/cloudwego/eino/adk"
)

// hostedMarketAgent and hostedMarketContext are shared fixtures for the
// gated hosted integration tests (full_multi_agent, full_stateless).
type hostedMarketAgent struct {
	result *market.Context
}

func (a *hostedMarketAgent) Analyze(
	_ context.Context,
	_ string,
	_ ...adk.AgentRunOption,
) (*market.Context, error) {
	return a.result, nil
}

func hostedMarketContext() *market.Context {
	return &market.Context{
		AsOf:   "20260923",
		Regime: market.RegimeMixed,
		Indexes: []market.IndexSnapshot{
			{
				Name:          "上证指数",
				ChangePercent: -0.39,
			},
			{
				Name:          "深证成指",
				ChangePercent: -0.64,
			},
			{
				Name:          "沪深300",
				ChangePercent: -0.60,
			},
		},
		AdvanceCount:   1891,
		DeclineCount:   3564,
		LimitUpCount:   51,
		LimitDownCount: 13,
		Turnover:       1_783_128_261_687,
		LeadingSectors: []market.SectorPerformance{
			{
				Name:          "半导体材料",
				ChangePercent: 2.91,
			},
		},
		Evidence: []string{
			"主要宽基指数小幅收跌，整体市场不是单边强势。",
			"上涨1891家、下跌3564家，市场广度明显偏弱。",
			"涨停51家、跌停13家，局部短线情绪仍有活跃度。",
			"市场存在结构性热点，但热点集中且轮动较快。",
		},
		Risk: []string{
			"指数跌幅与个股广度存在一定背离。",
			"结构性行情可能进一步收窄。",
		},
		Confidence: 0.64,
	}
}
