package skillruntime

import (
	"context"
	"fmt"
	"log"
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

	skills, err := skillBackend.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}

	for _, s := range skills {
		log.Printf(
			"[skill:discovered] name=%s description=%s",
			s.Name,
			s.Description,
		)
	}

	skillMW, err := skill.NewTyped[*schema.AgenticMessage](
		ctx,
		&skill.TypedConfig[*schema.AgenticMessage]{
			Backend: skillBackend,
			CustomSystemPrompt: func(ctx context.Context, toolName string) string {
				return fmt.Sprintf(`# Skill Runtime

在执行用户任务前，先检查可用 Skill 是否与当前任务匹配。

规则：

1. 如果存在明显匹配当前任务的 Skill，必须首先调用 %q 加载该 Skill。
2. 在相关 Skill 加载完成之前，不要直接使用其他领域工具绕过该 Skill。
3. Skill 加载后，按照 Skill 中定义的工作流、前置检查、脚本、参考资料和执行要求完成任务。
4. Skill 可以使用 filesystem、execute 以及其他可用工具完成任务。
5. 不要重复加载已经加载过的 Skill。

# Skill 运行依赖

当已加载的 Skill 缺少运行依赖时：

1. 先检查当前环境。
2. 如果依赖可以通过现有 execute 能力自行安装，则自行安装。
3. 安装后重新验证，并继续原任务。
4. 不要因为普通依赖缺失就停止任务。
5. 只有凭证、账号权限、付费权限或系统权限等无法自行获得的条件，
   才将其视为外部阻塞。`, toolName)
			},
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
