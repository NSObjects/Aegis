package overview

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type MarketOverview struct {
	Indexes        []IndexSnapshot     `json:"indexes"`
	AdvanceCount   int                 `json:"advance_count"`
	DeclineCount   int                 `json:"decline_count"`
	LimitUpCount   int                 `json:"limit_up_count"`
	LimitDownCount int                 `json:"limit_down_count"`
	Turnover       float64             `json:"turnover"`
	Sectors        []SectorPerformance `json:"sectors"`
}

type IndexSnapshot struct {
	Name          string  `json:"name"`
	ChangePercent float64 `json:"change_percent"`
}

type SectorPerformance struct {
	Name          string  `json:"name"`
	ChangePercent float64 `json:"change_percent"`
}

type Tool struct {
	Provider Provider
}

func (t *Tool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        "market_overview",
		Desc:        "获取当前 A 股整体市场环境，包括主要指数表现、涨跌家数、涨跌停数量、市场成交额和主要板块表现。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (t *Tool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	opts ...tool.Option,
) (string, error) {
	overview, err := t.Provider.Overview(ctx)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(overview)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
