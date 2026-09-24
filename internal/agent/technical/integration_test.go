package technical

import (
	technicalcap "Aegis/internal/capability/technical"
	"Aegis/internal/config"
	"Aegis/internal/llm"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/tool"
	callbacksHelper "github.com/cloudwego/eino/utils/callbacks"
)

func TestTechnicalAgent_Analyze_Integration(t *testing.T) {
	if os.Getenv("AEGIS_INTEGRATION") != "1" {
		t.Skip("set AEGIS_INTEGRATION=1 to run integration test")
	}

	if os.Getenv("DEEPSEEK_API_KEY") == "" {
		t.Fatal("DEEPSEEK_API_KEY is required")
	}

	if os.Getenv("TUSHARE_TOKEN") == "" {
		t.Fatal("TUSHARE_TOKEN is required")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()

	skillsDir, err := findSkillsDir()
	if err != nil {
		t.Fatal(err)
	}

	appCfg := config.Config{
		DefaultModel: "default",
		Models: map[string]config.ModelConfig{
			"default": {
				Provider: config.ProviderDeepSeek,
				Model: envOrDefault(
					"DEEPSEEK_MODEL",
					"deepseek-chat",
				),
				APIKeyEnv: "DEEPSEEK_API_KEY",
				BaseURL:   os.Getenv("DEEPSEEK_BASE_URL"),
			},
		},
	}

	profile, err := appCfg.ModelProfileForAgent("technical")
	if err != nil {
		t.Fatalf(
			"resolve technical model profile: %v",
			err,
		)
	}

	if profile != "default" {
		t.Fatalf(
			"technical model profile = %q, want %q",
			profile,
			"default",
		)
	}

	models := llm.NewManager(appCfg.Models)

	technicalModel, err := models.Get(
		ctx,
		profile,
	)
	if err != nil {
		t.Fatalf(
			"create technical model: %v",
			err,
		)
	}

	researcher, err := technicalcap.NewScriptResearcher(
		technicalcap.ScriptResearcherConfig{
			ScriptPath: filepath.Join(
				skillsDir,
				"tushare-data",
				"scripts",
				"stock_research.py",
			),
		},
	)
	if err != nil {
		t.Fatalf(
			"create technical researcher: %v",
			err,
		)
	}

	researchTool, err := technicalcap.NewTool(
		researcher,
	)
	if err != nil {
		t.Fatalf(
			"create technical research tool: %v",
			err,
		)
	}

	a, err := New(
		ctx,
		Config{
			Model: technicalModel,
			Tools: []tool.BaseTool{
				researchTool,
			},
			MaxIterations: 30,
		},
	)
	if err != nil {
		t.Fatalf(
			"create technical agent: %v",
			err,
		)
	}

	const (
		symbol        = "600519.SH"
		requestedAsOf = "20260923"
	)

	var (
		traceMu   sync.Mutex
		toolTrace []string
	)

	handler := callbacksHelper.NewHandlerHelper().
		Tool(&callbacksHelper.ToolCallbackHandler{
			OnStart: func(
				ctx context.Context,
				info *callbacks.RunInfo,
				input *tool.CallbackInput,
			) context.Context {
				if info == nil || input == nil {
					return ctx
				}

				traceMu.Lock()
				toolTrace = append(
					toolTrace,
					fmt.Sprintf(
						"%s input=%s",
						info.Name,
						input.ArgumentsInJSON,
					),
				)
				traceMu.Unlock()

				return ctx
			},
			OnError: func(
				ctx context.Context,
				info *callbacks.RunInfo,
				err error,
			) context.Context {
				if info == nil {
					return ctx
				}

				traceMu.Lock()
				toolTrace = append(
					toolTrace,
					fmt.Sprintf(
						"%s error=%v",
						info.Name,
						err,
					),
				)
				traceMu.Unlock()

				return ctx
			},
		}).
		Handler()

	result, err := a.Analyze(
		ctx,
		Input{
			Symbol:        symbol,
			RequestedAsOf: requestedAsOf,
		},
		adk.WithCallbacks(handler),
	)
	if err != nil {
		t.Fatalf(
			"analyze technical symbol: %v",
			err,
		)
	}

	if result == nil {
		t.Fatal("technical context is nil")
	}

	if result.Symbol != symbol {
		t.Fatalf(
			"technical symbol = %q, want %q",
			result.Symbol,
			symbol,
		)
	}

	if result.RequestedAsOf != requestedAsOf {
		t.Fatalf(
			"requested_as_of = %q, want %q",
			result.RequestedAsOf,
			requestedAsOf,
		)
	}

	if err := Validate(result); err != nil {
		t.Fatalf(
			"validate technical context: %v",
			err,
		)
	}

	if result.Status == StatusNormal {
		if result.EffectiveAsOf == "" {
			t.Fatal(
				"effective_as_of is empty for NORMAL status",
			)
		}

		if result.EffectiveAsOf > result.RequestedAsOf {
			t.Fatalf(
				"effective_as_of %q is after requested_as_of %q",
				result.EffectiveAsOf,
				result.RequestedAsOf,
			)
		}

		if result.Snapshot == nil {
			t.Fatal(
				"snapshot is nil for NORMAL status",
			)
		}
	}

	data, err := json.MarshalIndent(
		result,
		"",
		"  ",
	)
	if err != nil {
		t.Fatalf(
			"marshal technical context: %v",
			err,
		)
	}

	t.Logf(
		"technical context:\n%s",
		string(data),
	)

	traceMu.Lock()
	trace := append(
		[]string(nil),
		toolTrace...,
	)
	traceMu.Unlock()

	if len(trace) == 0 {
		t.Log(
			"tool trace is empty; TechnicalContext passed, but tool/capability execution path was not observed",
		)
		return
	}

	t.Log("tool trace:")

	foundResearchTool := false
	for _, entry := range trace {
		t.Log(entry)

		if strings.HasPrefix(
			entry,
			technicalcap.ToolName+" ",
		) {
			foundResearchTool = true
		}
	}

	if !foundResearchTool {
		t.Fatalf(
			"expected %s tool call, trace=%v",
			technicalcap.ToolName,
			trace,
		)
	}
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
