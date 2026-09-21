package skillruntime

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/schema"
)

func New(ctx context.Context) ([]adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage], error) {
	//home, err := os.UserHomeDir()
	//if err != nil {
	//	return nil, fmt.Errorf("get home dir: %w", err)
	//}
	//
	//root := filepath.Join(home, ".aegis", "skills")
	root := "./skills"
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create root dir: %w", err)
	}

	backend, err := local.NewBackend(ctx, &local.Config{})
	if err != nil {
		return nil, fmt.Errorf("create backend: %w", err)
	}

	skillBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: backend,
		BaseDir: root,
	})

	if err != nil {
		return nil, fmt.Errorf("create skill backend: %w", err)
	}

	skillMW, err := skill.NewTyped[*schema.AgenticMessage](
		ctx,
		&skill.TypedConfig[*schema.AgenticMessage]{
			Backend: skillBackend,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create skill middleware: %w", err)
	}
	fsMW, err := filesystem.NewTyped[*schema.AgenticMessage](ctx, &filesystem.MiddlewareConfig{
		Backend: backend,
		Shell:   backend,
		WriteFileToolConfig: &filesystem.ToolConfig{
			Disable: true,
		},
		EditFileToolConfig: &filesystem.ToolConfig{
			Disable: true,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("create filesystem middleware: %w", err)
	}

	return []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{
		skillMW,
		fsMW,
	}, nil
}
