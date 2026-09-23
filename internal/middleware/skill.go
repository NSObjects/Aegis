package middleware

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	einofs "github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/schema"
)

func NewSkill(
	ctx context.Context,
	backend einofs.Backend,
	skillsDir string,
) (
	adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage],
	error,
) {
	skillBackend, err := skill.NewBackendFromFilesystem(
		ctx,
		&skill.BackendFromFilesystemConfig{
			Backend: backend,
			BaseDir: skillsDir,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create skill backend: %w", err)
	}

	middleware, err := skill.NewTyped[*schema.AgenticMessage](
		ctx,
		&skill.TypedConfig[*schema.AgenticMessage]{
			Backend:            skillBackend,
			CustomSystemPrompt: skillSystemPrompt,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create skill middleware: %w", err)
	}

	return middleware, nil
}

func skillSystemPrompt(
	_ context.Context,
	toolName string,
) string {
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
5. 只有凭证、账号权限、付费权限或系统权限等无法自行获得的条件，才将其视为外部阻塞。
6. 不得伪造 Token、账号权限或数据。`, toolName)
}
