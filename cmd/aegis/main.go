package main

import (
	"Aegis/internal/decision"
	"Aegis/internal/runtime/mcp"
	"Aegis/internal/tool/news"
	"Aegis/internal/tool/overview"
	"Aegis/internal/tool/technical"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/agenticdeepseek"
	"github.com/cloudwego/eino-ext/devops"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	callbacksHelper "github.com/cloudwego/eino/utils/callbacks"
)

func main() {
	ctx := context.Background()

	mcpConfigs, err := mcp.LoadConfig("configs/mcp.json")
	if err != nil {
		log.Printf("[mcp] load config failed: %v", err)
	}

	mcpManager, err := mcp.ConnectAll(ctx, mcpConfigs)
	if err != nil {
		log.Fatalf("[mcp] connect failed: %v", err)
	}
	defer func() {
		if err := mcpManager.Close(); err != nil {
			log.Printf("[mcp] close failed: %v", err)
		}
	}()

	for name, err := range mcpManager.Failures() {
		log.Printf(
			"[mcp:error] server=%s err=%v",
			name,
			err,
		)
	}

	//for _, t := range mcpManager.Tools() {
	//	info, err := t.Info(ctx)
	//	if err != nil {
	//		log.Printf("[mcp:error] tool info: %v", err)
	//		continue
	//	}
	//
	//	log.Printf(
	//		"[mcp:tool] name=%s desc=%s",
	//		info.Name,
	//		info.Desc,
	//	)
	//}
	handler := callbacksHelper.NewHandlerHelper().
		Tool(&callbacksHelper.ToolCallbackHandler{
			OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
				log.Printf("[tool:start] %s", info.Name)
				return ctx
			},
			OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *tool.CallbackOutput) context.Context {
				log.Printf("[tool:end] %s", info.Name)
				return ctx
			},
			OnError: func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
				log.Printf("[tool:error] %s: %v", info.Name, err)
				return ctx
			},
		}).
		Handler()

	if err := devops.Init(ctx); err != nil {
		log.Fatal(err)
	}
	dp, err := agenticdeepseek.New(ctx, &agenticdeepseek.Config{
		APIKey:             "sk-26dc82455e6e437cb32f97c2f25c4c41",
		BaseURL:            "https://api.deepseek.com",
		Model:              "deepseek-flash",
		ResponseFormatType: agenticdeepseek.ResponseFormatTypeJSONObject,
	})

	if err != nil {
		panic(err)
	}
	scenario := "bullish"
	tls := tools(scenario)

	tls = append(tls, mcpManager.Tools()...)
	agent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](
		ctx,
		&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:        "aegis",
			Description: "Aegis trading research agent",
			Instruction: `
你是 Aegis，一个交易研究 Agent。

你的任务是进行交易研究，而不是执行交易。

分析个股短线状态时，必须结合整体市场环境。

整体市场环境包括指数表现、涨跌家数、涨跌停数量、
市场成交额和板块表现。

这些数据是客观事实，不是交易结论。
你需要自行判断整体市场环境如何影响当前个股的短线机会和风险。

不要简单复述市场数据，要解释市场环境与个股状态之间的关系。

当用户要求分析某个标的时：

1. 判断当前问题需要哪些信息。
2. 必要时调用可用工具获取信息。
3. 根据工具返回的数据进行综合分析。
4. 如果信息不足，继续调用必要的工具，不要猜测。
5. 如果不同工具返回的数据存在冲突，必须明确指出冲突，并降低判断置信度。
6. 最终必须输出一个结构化 Decision。

在进行交易研究前，先判断本次任务的时间意图，并确定分析时间范围。

所有具有时间属性的数据，都必须与本次任务的目标时间范围一致。

规则：

1. 根据用户的研究意图确定目标时间，而不是依赖“当前”、“最近”、“短线”等固定关键词。
2. 如果任务是在评估标的当前的交易状态、机会、风险或候选价值，应以当前分析时间和当前有效交易日为时间基准。
3. 如果任务明确针对历史时期、历史行情或指定日期，应使用对应历史时间范围的数据。
4. 调用行情、资金流、新闻、技术指标等工具时，应根据目标时间范围传递合适的时间参数。
5. 不得将不同时间基准的数据直接当作同一时点的数据进行比较。
6. 如果工具返回的数据与本次任务的目标时间范围不一致，应优先重新获取正确时间范围的数据。
7. 如果无法获取符合目标时间范围的数据，应明确说明数据缺失，不得使用无关或过期数据替代。


Decision 必须包含：

* symbol：股票代码，string
* market_view：当前市场/短线状态判断，string
* evidence：支持判断的主要证据，string[]
* risk：当前主要风险，string[]
* action：当前建议的状态，string
* confidence：对当前判断的置信度，number

action 只能使用：

* WATCH：继续观察
* AVOID：当前不考虑
* CANDIDATE：进入短线候选池

confidence 必须是 0 到 1 之间的小数。

evidence 和 risk 必须始终输出为 JSON 字符串数组。
即使只有一条 evidence 或 risk，也必须使用数组，不能输出为单个字符串。

正确示例：

{
"symbol": "600519",
"market_view": "当前短线状态判断",
"evidence": [
"证据1",
"证据2"
],
"risk": [
"风险1",
"风险2"
],
"action": "WATCH",
"confidence": 0.6
}

不要输出 BUY 或 SELL。
不要因为用户要求买入或卖出，就直接给出交易执行建议。

最终输出必须是合法 JSON，不要使用 Markdown 代码块，不要在 JSON 前后添加其他文字。

`,
			Model: dp,
			ToolsConfig: adk.ToolsConfig{
				ToolsNodeConfig: compose.ToolsNodeConfig{
					Tools: tls,
				},
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	runner := adk.NewTypedRunner[*schema.AgenticMessage](
		adk.TypedRunnerConfig[*schema.AgenticMessage]{
			Agent:           agent,
			EnableStreaming: true,
		},
	)

	events := runner.Run(
		ctx,
		[]*schema.AgenticMessage{
			schema.UserAgenticMessage("分析一下 600519 当前短线状态。"),
		},
		adk.WithCallbacks(handler),
	)
	var output strings.Builder
	for {
		event, ok := events.Next()
		if !ok {
			break
		}

		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}

		msg, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			log.Fatal(err)
		}

		if msg == nil {
			continue
		}

		for _, block := range msg.ContentBlocks {
			if block.AssistantGenText != nil {
				output.WriteString(block.AssistantGenText.Text)
			}
		}
	}

	var d decision.Decision

	if err := json.Unmarshal([]byte(output.String()), &d); err != nil {
		log.Fatalf("invalid decision JSON: %v\noutput: %s", err, output.String())
	}

	if err := decision.Validate(&d); err != nil {
		log.Fatalf("invalid decision: %v", err)
	}

	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(data))
}

func tools(scenario string) []tool.BaseTool {
	var tt []tool.BaseTool
	switch scenario {
	case "bullish":
		//tt = append(tt, &market.SnapshotTool{
		//	Data: market.Snapshot{
		//		Price:         1510,
		//		ChangePercent: 4.2,
		//		Turnover:      2800000000,
		//	},
		//})

		tt = append(tt, &technical.Tool{
			Data: technical.Indicators{
				MA5:           1482,
				MA20:          1455,
				MA60:          1410,
				RSI14:         66,
				MACD:          18,
				MACDSignal:    11,
				MACDHistogram: 7,
			},
		})

		tt = append(tt, &news.Tool{
			News: []news.News{
				{
					Title:       "行业政策边际改善",
					PublishedAt: "2026-09-20T10:30:00+08:00",
					Source:      "示例财经",
					Summary:     "行业政策环境出现积极变化，市场预期有所改善。",
				},
				{
					Title:       "行业龙头业绩超预期",
					PublishedAt: "2026-09-20T09:15:00+08:00",
					Source:      "示例财经",
					Summary:     "行业龙头最新经营数据明显好于市场预期。",
				},
			},
		})
		overviewProvider := &overview.StaticProvider{
			Data: overview.MarketOverview{
				Indexes: []overview.IndexSnapshot{
					{Name: "上证指数", ChangePercent: 1.52},
					{Name: "深证成指", ChangePercent: 2.13},
					{Name: "沪深300", ChangePercent: 1.36},
				},
				AdvanceCount:   4210,
				DeclineCount:   820,
				LimitUpCount:   86,
				LimitDownCount: 9,
				Turnover:       1_850_000_000_000,
				Sectors: []overview.SectorPerformance{
					{Name: "商业航天", ChangePercent: 6.8},
					{Name: "机器人", ChangePercent: 5.2},
					{Name: "半导体", ChangePercent: 3.7},
				},
			},
		}
		tt = append(tt, &overview.Tool{Provider: overviewProvider})
	case "conflict":
		//tt = append(tt, &market.SnapshotTool{
		//	Data: market.Snapshot{
		//		Price:         1450.20,
		//		ChangePercent: 2.31,
		//		Turnover:      1850000000,
		//	},
		//})

		tt = append(tt, &technical.Tool{
			Data: technical.Indicators{
				MA5:           1482.30,
				MA20:          1465.80,
				MA60:          1418.50,
				RSI14:         58.6,
				MACD:          12.8,
				MACDSignal:    9.4,
				MACDHistogram: 3.4,
			},
		})

		tt = append(tt, &news.Tool{
			News: []news.News{
				{
					Title:       "行业政策出现收紧信号",
					PublishedAt: "2026-09-20T10:30:00+08:00",
					Source:      "示例财经",
					Summary:     "相关政策变化可能压制行业短期预期。",
				},
				{
					Title:       "行业龙头经营数据低于预期",
					PublishedAt: "2026-09-20T09:15:00+08:00",
					Source:      "示例财经",
					Summary:     "最新经营数据弱于此前市场预期。",
				},
			},
		})

	case "bearish":
		//tt = append(tt, &market.SnapshotTool{
		//	Data: market.Snapshot{
		//		Price:         1380,
		//		ChangePercent: -3.8,
		//		Turnover:      2200000000,
		//	},
		//})

		tt = append(tt, &technical.Tool{
			Data: technical.Indicators{
				MA5:           1420,
				MA20:          1455,
				MA60:          1480,
				RSI14:         32,
				MACD:          -18,
				MACDSignal:    -10,
				MACDHistogram: -8,
			},
		})

		tt = append(tt, &news.Tool{
			News: []news.News{
				{
					Title:       "行业政策进一步收紧",
					PublishedAt: "2026-09-20T10:30:00+08:00",
					Source:      "示例财经",
					Summary:     "行业监管政策进一步收紧，市场短期预期明显转弱。",
				},
				{
					Title:       "行业龙头业绩低于预期",
					PublishedAt: "2026-09-20T09:15:00+08:00",
					Source:      "示例财经",
					Summary:     "行业龙头最新经营数据明显低于市场预期。",
				},
			},
		})

	default:
		log.Fatalf("unknown scenario: %s", scenario)
	}

	return tt
}
