package technical

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type Indicators struct {
	Symbol        string  `json:"symbol"`
	MA5           float64 `json:"ma5"`
	MA20          float64 `json:"ma20"`
	MA60          float64 `json:"ma60"`
	RSI14         float64 `json:"rsi14"`
	MACD          float64 `json:"macd"`
	MACDSignal    float64 `json:"macd_signal"`
	MACDHistogram float64 `json:"macd_histogram"`
}

type Tool struct {
	Data Indicators
}

func (t *Tool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "technical_indicators",
		Desc: "获取指定股票的主要技术指标，用于判断短期趋势、均线结构和动量状态。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"symbol": {
				Type:     schema.String,
				Desc:     "股票代码，例如 600519",
				Required: true,
			},
		}),
	}, nil
}
func (t *Tool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	opts ...tool.Option,
) (string, error) {
	var args struct {
		Symbol string `json:"symbol"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", err
	}

	data := t.Data
	data.Symbol = args.Symbol

	result, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(result), nil
}
