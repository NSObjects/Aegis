package market

import (
	"Aegis/internal/runtime"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudwego/eino-ext/components/model/agenticdeepseek"
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

	rt, err := runtime.New(
		ctx,
		runtime.Config{
			SkillsDir: skillsDir,
			TimeZone:  "Asia/Shanghai",
		},
	)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}

	a, err := New(
		ctx,
		Config{
			Model:    dp,
			Handlers: rt.Handlers(),

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
