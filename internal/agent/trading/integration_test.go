package trading

import (
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/config"
	"Aegis/internal/decision"
	"Aegis/internal/llm"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestTradingAgent_Analyze_Integration(t *testing.T) {
	if os.Getenv("AEGIS_INTEGRATION") != "1" {
		t.Skip("set AEGIS_INTEGRATION=1 to run integration test")
	}

	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Fatal("DEEPSEEK_API_KEY is required")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()

	appCfg := config.Config{
		DefaultModel: "default",
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:  config.ProviderDeepSeek,
				Model:     "deepseek-chat",
				APIKeyEnv: "DEEPSEEK_API_KEY",
			},
			"strong": {
				Provider: config.ProviderDeepSeek,
				Model: envOrDefault(
					"DEEPSEEK_STRONG_MODEL",
					"deepseek-chat",
				),
				APIKeyEnv: "DEEPSEEK_API_KEY",
				BaseURL:   os.Getenv("DEEPSEEK_BASE_URL"),
			},
		},
		Agents: map[string]config.AgentConfig{
			"trading": {
				Model: "strong",
			},
		},
	}

	profile, err := appCfg.ModelProfileForAgent("trading")
	if err != nil {
		t.Fatalf("resolve trading model profile: %v", err)
	}

	if profile != "strong" {
		t.Fatalf(
			"trading model profile = %q, want %q",
			profile,
			"strong",
		)
	}

	models := llm.NewManager(appCfg.Models)

	tradingModel, err := models.Get(ctx, profile)
	if err != nil {
		t.Fatalf("create trading model: %v", err)
	}

	a, err := New(
		ctx,
		Config{
			Model:         tradingModel,
			MaxIterations: 10,
		},
	)
	if err != nil {
		t.Fatalf("create trading agent: %v", err)
	}

	result, err := a.Analyze(
		ctx,
		Input{
			Symbol:           "600519.SH",
			MarketContext:    integrationMarketContext(),
			TechnicalContext: integrationTechnicalContext(),
		},
	)
	if err != nil {
		t.Fatalf("analyze trading symbol: %v", err)
	}

	if result == nil {
		t.Fatal("trading decision is nil")
	}

	if result.Symbol != "600519.SH" {
		t.Fatalf(
			"decision symbol = %q, want %q",
			result.Symbol,
			"600519.SH",
		)
	}

	if err := decision.Validate(result); err != nil {
		t.Fatalf("validate trading decision: %v", err)
	}

	data, err := json.MarshalIndent(
		result,
		"",
		"  ",
	)
	if err != nil {
		t.Fatalf("marshal trading decision: %v", err)
	}

	t.Logf(
		"trading decision:\n%s",
		string(data),
	)
}

func integrationMarketContext() *market.Context {
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
				Name:          "创业板指",
				ChangePercent: -0.60,
			},
			{
				Name:          "沪深300",
				ChangePercent: -0.60,
			},
			{
				Name:          "中证1000",
				ChangePercent: -0.17,
			},
		},
		AdvanceCount:   1891,
		DeclineCount:   3564,
		LimitUpCount:   51,
		LimitDownCount: 13,
		Turnover:       1_783_128_261_687,
		LeadingSectors: []market.SectorPerformance{
			{
				Name:          "商用载货车",
				ChangePercent: 3.97,
			},
			{
				Name:          "房地产服务",
				ChangePercent: 3.31,
			},
			{
				Name:          "半导体材料",
				ChangePercent: 2.91,
			},
		},
		Evidence: []string{
			"主要宽基指数小幅收跌，市场并非单边风险偏好抬升。",
			"全市场上涨1891家、下跌3564家，市场广度明显偏弱。",
			"成交额约1.78万亿元，较前一交易日缩量约17%。",
			"涨停51家、跌停13家，局部短线情绪仍有活跃度。",
			"电子链和地产链存在结构性热点。",
		},
		Risk: []string{
			"指数跌幅与个股广度存在背离。",
			"结构性行情可能进一步收窄。",
			"市场热点集中，轮动速度较快。",
		},
		Confidence: 0.64,
	}
}

func integrationTechnicalContext() *technical.Context {
	return &technical.Context{
		Symbol:          "600519.SH",
		RequestedAsOf:   "20260923",
		EffectiveAsOf:   "20260923",
		LastTradeDate:   "20260923",
		Status:          technical.StatusNormal,
		Snapshot: &technical.Snapshot{
			Open:          1255.03,
			High:          1271.50,
			Low:           1250.89,
			Close:         1251.24,
			PreClose:      1253.80,
			ChangePercent: -0.2042,
			Volume:        3_098_122,
			Amount:        3_894_630_783,
		},
		Returns: technical.Returns{
			Return1D:  float64Ptr(-0.2042),
			Return5D:  float64Ptr(-0.5374),
			Return10D: float64Ptr(-3.0708),
			Return20D: float64Ptr(-3.9576),
			Return60D: float64Ptr(4.8809),
		},
		MovingAverages: technical.MovingAverages{
			MA5:  float64Ptr(1256.342),
			MA10: float64Ptr(1265.071),
			MA20: float64Ptr(1284.103),
			MA60: float64Ptr(1285.7557),
		},
		Momentum: technical.Momentum{
			RSI14: float64Ptr(37.0889),
			MACD: technical.MACD{
				DIF:  float64Ptr(-12.9561),
				DEA:  float64Ptr(-8.4392),
				HIST: float64Ptr(-9.0337),
			},
		},
		Volume: technical.VolumeMetrics{
			VolumeMA5:    float64Ptr(2_460_314.4),
			VolumeMA20:   float64Ptr(2_442_862.4),
			VolumeVsMA5:  float64Ptr(1.2592),
			VolumeVsMA20: float64Ptr(1.2682),
			AmountMA5:    float64Ptr(3_094_450_873.4),
			AmountMA20:   float64Ptr(3_140_884_123.05),
			AmountVsMA5:  float64Ptr(1.2586),
			AmountVsMA20: float64Ptr(1.24),
		},
		Evidence: []string{
			"收盘价低于 MA5、MA10、MA20 与 MA60，主要均线呈空头排列。",
			"MACD 位于负值区间，DIF 低于 DEA，短中期动能偏弱。",
			"5日、10日和20日收益均为负，60日收益仍为正，存在周期冲突。",
			"RSI14 为 37.0889，偏弱但尚未进入超卖区域。",
			"成交量约为 5 日和 20 日均量的 1.26 倍，价跌量增需要后续确认。",
		},
		Risk: []string{
			"短中期弱势与60日正收益存在方向冲突。",
			"价跌量增可能反映卖压，但单日量能不足以确认趋势延续。",
		},
		Confidence: 0.82,
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}

func envOrDefault(
	name string,
	fallback string,
) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}
