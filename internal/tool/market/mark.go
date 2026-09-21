package market

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type SnapshotTool struct {
	Data Snapshot
}

type Snapshot struct {
	Symbol        string  `json:"symbol"`
	Price         float64 `json:"price"`
	ChangePercent float64 `json:"change_percent"`
	Turnover      float64 `json:"turnover"`
}

func (t *SnapshotTool) InvokableRun(
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

func (t *SnapshotTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "market_snapshot",
		Desc: "获取指定股票当前的市场快照，包括价格、涨跌幅、成交额等信息。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"symbol": {
				Type:     schema.String,
				Desc:     "股票代码，例如 600519",
				Required: true,
			},
		}),
	}, nil
}
