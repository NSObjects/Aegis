package news

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type News struct {
	Title       string `json:"title"`
	PublishedAt string `json:"published_at"`
	Source      string `json:"source"`
	Summary     string `json:"summary"`
}

type Tool struct {
	News []News
}

func (t *Tool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "news_search",
		Desc: "获取指定股票近期相关新闻、公告和可能影响短期走势的事件信息。",
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

	result := struct {
		Symbol string `json:"symbol"`
		News   []News `json:"news"`
	}{
		Symbol: args.Symbol,
		News:   t.News,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
