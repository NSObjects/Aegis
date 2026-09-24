package event

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

const ToolName = "stock_event_research"

func NewTool(
	researcher Researcher,
) (tool.InvokableTool, error) {
	if researcher == nil {
		return nil, errors.New(
			"event researcher is required",
		)
	}

	return toolutils.InferTool[Request, *Result](
		ToolName,
		"Collect point-in-time event evidence for one A-share symbol from multiple configured sources. The result contains explicit per-source coverage and raw items. Treat missing or failed coverage separately from NO_EVENT, and do not claim sources that were not queried.",
		func(
			ctx context.Context,
			req Request,
		) (*Result, error) {
			return researcher.Research(
				ctx,
				req,
			)
		},
	)
}
