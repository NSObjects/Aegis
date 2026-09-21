package main

import (
	"Aegis/internal/decision"
	"Aegis/internal/tool/market"
	"Aegis/internal/tool/news"
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
	scenario := "bearish"

	agent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](
		ctx,
		&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:        "aegis",
			Description: "Aegis trading research agent",
			Instruction: `
你是 Aegis，一个交易研究 Agent。

你的任务是进行交易研究，而不是执行交易。

当用户要求分析某个标的时：

1. 判断当前问题需要哪些信息。
2. 必要时调用可用工具获取信息。
3. 根据工具返回的数据进行综合分析。
4. 如果信息不足，继续调用必要的工具，不要猜测。
5. 最终必须输出一个结构化 Decision。

Decision 必须包含：

- symbol：股票代码
- market_view：当前市场/短线状态判断
- evidence：支持判断的主要证据
- risk：当前主要风险
- action：当前建议的状态
- confidence：对当前判断的置信度

action 只能使用：

- WATCH：继续观察
- AVOID：当前不考虑
- CANDIDATE：进入短线候选池

confidence 必须是 0 到 1 之间的小数。

不要输出 BUY 或 SELL。
不要因为用户要求买入或卖出，就直接给出交易执行建议。

最终输出必须是合法 JSON，不要使用 Markdown 代码块，不要在 JSON 前后添加其他文字。
`,
			Model: dp,
			ToolsConfig: adk.ToolsConfig{
				ToolsNodeConfig: compose.ToolsNodeConfig{
					Tools: tools(scenario),
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
		tt = append(tt, &market.SnapshotTool{
			Data: market.Snapshot{
				Price:         1510,
				ChangePercent: 4.2,
				Turnover:      2800000000,
			},
		})

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

	case "conflict":
		tt = append(tt, &market.SnapshotTool{
			Data: market.Snapshot{
				Price:         1450.20,
				ChangePercent: 2.31,
				Turnover:      1850000000,
			},
		})

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
		tt = append(tt, &market.SnapshotTool{
			Data: market.Snapshot{
				Price:         1380,
				ChangePercent: -3.8,
				Turnover:      2200000000,
			},
		})

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
