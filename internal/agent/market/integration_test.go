package market

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarketAgent_Analyze_Integration(t *testing.T) {
	//if os.Getenv("AEGIS_INTEGRATION") != "1" {
	//	t.Skip("set AEGIS_INTEGRATION=1 to run integration test")
	//}
	//
	//requireEnv(t, "DEEPSEEK_API_KEY")
	//requireEnv(t, "TUSHARE_TOKEN")
	//
	//ctx, cancel := context.WithTimeout(
	//	context.Background(),
	//	3*time.Minute,
	//)
	//defer cancel()
	//
	//skillsDir := findSkillsDir(t)
	//
	////
	//// 1. Application model configuration
	////
	//// Market Agent intentionally has no dedicated model configuration.
	//// It must inherit default_model.
	////
	//appCfg := config.Config{
	//	DefaultModel: "default",
	//
	//	Models: map[string]config.ModelConfig{
	//		"default": {
	//			Provider: config.ProviderDeepSeek,
	//
	//			Model: envOrDefault(
	//				"DEEPSEEK_MODEL",
	//				"deepseek-chat",
	//			),
	//
	//			APIKey: "DEEPSEEK_API_KEY",
	//
	//			BaseURL: os.Getenv(
	//				"DEEPSEEK_BASE_URL",
	//			),
	//		},
	//	},
	//}
	//
	////
	//// 2. Resolve model profile for Market Agent
	////
	//profile, err :=
	//	appCfg.ModelProfileForAgent("market")
	//if err != nil {
	//	t.Fatalf(
	//		"resolve market model profile: %v",
	//		err,
	//	)
	//}
	//
	//if profile != "default" {
	//	t.Fatalf(
	//		"market model profile = %q, want %q",
	//		profile,
	//		"default",
	//	)
	//}
	//
	////
	//// 3. Create/reuse model through LLM Manager
	////
	//models := llm.NewManager(
	//	appCfg.Models,
	//)
	//
	//marketModel, err :=
	//	models.Get(ctx, profile)
	//if err != nil {
	//	t.Fatalf(
	//		"create market model: %v",
	//		err,
	//	)
	//}
	//
	////
	//// 4. Create shared Aegis Runtime
	////
	//rt, err := runtime.New(
	//	ctx,
	//	runtime.Config{
	//		SkillsDir: skillsDir,
	//		TimeZone:  "Asia/Shanghai",
	//	},
	//)
	//if err != nil {
	//	t.Fatalf(
	//		"create runtime: %v",
	//		err,
	//	)
	//}
	//
	////
	//// 5. Assemble Market Agent
	////
	//a, err := New(
	//	ctx,
	//	Config{
	//		Model: marketModel,
	//
	//		Handlers: rt.AgentHandlers(),
	//
	//		MaxIterations: 30,
	//	},
	//)
	//if err != nil {
	//	t.Fatalf(
	//		"create market agent: %v",
	//		err,
	//	)
	//}
	//
	////
	//// 6. Execute real market research
	////
	//result, err := a.Analyze(
	//	ctx,
	//	"分析当前 A 股市场环境。",
	//	adk.WithCallbacks(
	//		newTestCallback(t),
	//	),
	//)
	//if err != nil {
	//	t.Fatalf(
	//		"analyze market: %v",
	//		err,
	//	)
	//}
	//
	//if result == nil {
	//	t.Fatal("market result is nil")
	//}
	//
	////
	//// Analyze() itself already performs structural validation.
	//// Integration test only keeps a few fundamental assertions here.
	////
	//if result.AsOf == "" {
	//	t.Fatal("market context as_of is empty")
	//}
	//
	//if len(result.Evidence) == 0 {
	//	t.Fatal("market context evidence is empty")
	//}
	//
	////
	//// 7. Print result for integration-test inspection
	////
	//data, err := json.MarshalIndent(
	//	result,
	//	"",
	//	"  ",
	//)
	//if err != nil {
	//	t.Fatalf(
	//		"marshal market context: %v",
	//		err,
	//	)
	//}
	//
	//t.Logf(
	//	"market context:\n%s",
	//	data,
	//)
}

func requireEnv(
	t *testing.T,
	name string,
) string {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		t.Skipf(
			"%s is required for integration test",
			name,
		)
	}

	return value
}

func envOrDefault(
	name string,
	fallback string,
) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

func findSkillsDir(
	t *testing.T,
) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf(
			"get working directory: %v",
			err,
		)
	}

	for {
		goMod := filepath.Join(
			dir,
			"go.mod",
		)

		if _, err := os.Stat(goMod); err == nil {
			skillsDir := filepath.Join(
				dir,
				"skills",
			)

			if _, err := os.Stat(skillsDir); err != nil {
				t.Fatalf(
					"skills directory not found: %s",
					skillsDir,
				)
			}

			return skillsDir
		}

		parent := filepath.Dir(dir)

		if parent == dir {
			t.Fatal(
				"repository root containing go.mod not found",
			)
		}

		dir = parent
	}
}
