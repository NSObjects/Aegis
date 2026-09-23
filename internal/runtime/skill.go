package runtime

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/schema"
)

func newSkillMiddleware(ctx context.Context, backend *local.Local, skillsDir string) (adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage], error) {
	skillBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: backend,
		BaseDir: skillsDir,
	})

	if err != nil {
		return nil, fmt.Errorf("create skill backend: %w", err)
	}

	middleware, err := skill.NewTyped[*schema.AgenticMessage](ctx, &skill.TypedConfig[*schema.AgenticMessage]{
		Backend:            skillBackend,
		CustomSystemPrompt: skillRuntimePrompt,
	})

	if err != nil {
		return nil, fmt.Errorf(
			"create runtime skill middleware: %w",
			err,
		)
	}

	return middleware, nil
}

func skillRuntimePrompt(
	_ context.Context,
	toolName string,
) string {
	return fmt.Sprintf(
		`# Skill Runtime

执行任务前，检查可用 Skill 是否与当前任务相关。

如果存在明显适合当前任务的 Skill：

1. 首先调用 %q 工具加载它。
2. 在 Skill 加载完成之前，不要绕过 Skill 直接执行领域任务。
3. 加载后遵循 Skill 中定义的工作流、前置检查、脚本、参考资料和执行要求。
4. Skill 可以使用 filesystem、execute 和其他当前可用工具。
5. 不要重复加载已经加载过的 Skill。

当 Skill 存在运行依赖时：

1. 先检查当前运行环境。
2. 如果缺少的是普通运行依赖，并且可以通过 execute 自动安装，则自行安装。
3. 安装完成后重新验证依赖，并继续原始任务。
4. 不要因为普通依赖缺失而停止任务。
5. 只有凭证、账号权限、付费权限或系统权限等无法自行获得的条件，才视为外部阻塞。
6. 不得伪造 Token、账号权限或数据。`,
		toolName,
	)
}
