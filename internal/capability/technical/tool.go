package technical

import (
	"context"
	"errors"
 "time"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

const ToolName = "stock_technical_research"

func NewTool(
	researcher Researcher,
) (tool.InvokableTool, error) {
	if researcher == nil {
		return nil, errors.New(
			"technical researcher is required",
		)
	}

	return toolutils.InferTool[Request, map[string]any](
		ToolName,
		"Fetch deterministic point-in-time stock technical research data for one symbol and as-of date. Use this as the authoritative source for OHLCV, PIT-adjusted returns, MA, RSI, MACD and volume metrics. Do not recalculate these metrics with ad-hoc code.",
		func(
			ctx context.Context,
			req Request,
		) (map[string]any, error) {
            started := time.Now()
            reportTool(ctx, ToolObservation{Status: "START", At: started.UTC()})
            result, err := researcher.Research(ctx, req)
            status := "DONE"
            if err != nil { status = "FAIL" }
            reportTool(ctx, ToolObservation{
                Status: status, At: time.Now().UTC(), DurationMS: time.Since(started).Milliseconds(),
            })
            return result, err
		},
	)
}
