package middleware

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	einofs "github.com/cloudwego/eino/adk/filesystem"
	filesystemmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/schema"
)

func NewFilesystem(
	ctx context.Context,
	backend einofs.Backend,
	shell einofs.Shell,
) (
	adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage],
	error,
) {
	middleware, err := filesystemmw.NewTyped[*schema.AgenticMessage](
		ctx,
		&filesystemmw.MiddlewareConfig{
			Backend: backend,
			Shell:   shell,

			WriteFileToolConfig: &filesystemmw.ToolConfig{
				Disable: true,
			},
			EditFileToolConfig: &filesystemmw.ToolConfig{
				Disable: true,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create filesystem middleware: %w", err)
	}

	return middleware, nil
}
