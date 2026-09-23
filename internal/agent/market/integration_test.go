package market

import (
	"Aegis/internal/runtime"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino-ext/components/model/agenticdeepseek"
	"github.com/cloudwego/eino/adk"
	fsmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/schema"
)

func TestMarketAgent_Analyze_Integration(t *testing.T) {
	if os.Getenv("AEGIS_INTEGRATION") != "1" {
		t.Skip("set AEGIS_INTEGRATION=1 to run integration test")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()

	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Fatal("DEEPSEEK_API_KEY is required")
	}

	if os.Getenv("TUSHARE_TOKEN") == "" {
		t.Fatal("TUSHARE_TOKEN is required")
	}

	skillsDir, err := findSkillsDir()
	if err != nil {
		t.Fatal(err)
	}

	dp, err := newIntegrationModel(
		ctx,
		apiKey,
	)
	if err != nil {
		t.Fatalf("create model: %v", err)
	}

	handlers, err := newIntegrationHandlers(
		ctx,
		skillsDir,
	)
	if err != nil {
		t.Fatalf("create handlers: %v", err)
	}

	a, err := New(
		ctx,
		Config{
			Model:    dp,
			Handlers: handlers,

			MaxIterations: 30,
		},
	)
	if err != nil {
		t.Fatalf("create market agent: %v", err)
	}

	result, err := a.Analyze(
		ctx,
		"分析当前 A 股市场环境。",
	)
	if err != nil {
		t.Fatalf("analyze market: %v", err)
	}

	data, _ := json.MarshalIndent(
		result,
		"",
		"  ",
	)

	t.Logf(
		"market context:\n%s",
		string(data),
	)
}

func newIntegrationModel(
	ctx context.Context,
	apiKey string,
) (*agenticdeepseek.Model, error) {
	modelName := os.Getenv("DEEPSEEK_MODEL")
	if modelName == "" {
		modelName = "deepseek-chat"
	}

	cfg := &agenticdeepseek.Config{
		APIKey: apiKey,
		Model:  modelName,

		Timeout: 120 * time.Second,

		ResponseFormatType: agenticdeepseek.ResponseFormatTypeJSONObject,
	}

	if baseURL := os.Getenv("DEEPSEEK_BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	}

	return agenticdeepseek.New(
		ctx,
		cfg,
	)
}

func newIntegrationHandlers(
	ctx context.Context,
	skillsDir string,
) (
	[]adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage],
	error,
) {
	backend, err := local.NewBackend(
		ctx,
		&local.Config{},
	)
	if err != nil {
		return nil, err
	}

	skillBackend, err := skill.NewBackendFromFilesystem(
		ctx,
		&skill.BackendFromFilesystemConfig{
			Backend: backend,
			BaseDir: skillsDir,
		},
	)
	if err != nil {
		return nil, err
	}

	skillMW, err := skill.NewTyped[*schema.AgenticMessage](
		ctx,
		&skill.TypedConfig[*schema.AgenticMessage]{
			Backend: skillBackend,

			CustomSystemPrompt: skillRuntimePrompt,
		},
	)
	if err != nil {
		return nil, err
	}

	fsMW, err := fsmw.NewTyped[*schema.AgenticMessage](
		ctx,
		&fsmw.MiddlewareConfig{
			Backend: backend,
			Shell:   backend,

			WriteFileToolConfig: &fsmw.ToolConfig{
				Disable: true,
			},

			EditFileToolConfig: &fsmw.ToolConfig{
				Disable: true,
			},
		},
	)
	if err != nil {
		return nil, err
	}
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, err
	}

	contextMW :=
		runtime.NewContextMiddleware[*schema.AgenticMessage](
			shanghai,
		)
	return []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{
		skillMW,
		fsMW,
		contextMW,
	}, nil
}

func skillRuntimePrompt(
	_ context.Context,
	toolName string,
) string {
	return `
# Skill Runtime

执行任务前，检查可用 Skill 是否与当前任务相关。

如果存在明显适合当前任务的 Skill：

1. 首先调用 Skill 工具加载它。
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
6. 不得伪造 Token、账号权限或数据。
`
}

func findSkillsDir() (string, error) {
	if dir := os.Getenv("AEGIS_SKILLS_DIR"); dir != "" {
		return filepath.Abs(dir)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		goMod := filepath.Join(
			dir,
			"go.mod",
		)

		if _, err := os.Stat(goMod); err == nil {
			return filepath.Join(
				dir,
				"skills",
			), nil
		}

		parent := filepath.Dir(dir)

		if parent == dir {
			return "", os.ErrNotExist
		}

		dir = parent
	}
}
