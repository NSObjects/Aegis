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

	return toolutils.InferTool[Request, map[string]any](
		ToolName,
		"Fetch point-in-time event facts for one A-share symbol and as-of date. The current implementation covers listed-company announcements only and returns explicit coverage metadata. Do not claim news or policy coverage when those sources are not covered.",
		func(
			ctx context.Context,
			req Request,
		) (map[string]any, error) {
			return researcher.Research(
				ctx,
				req,
			)
		},
	)
}
