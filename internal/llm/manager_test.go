package llm

import (
	"Aegis/internal/config"
	"context"
	"strings"
	"testing"
)

func TestAPIKey(t *testing.T) {
	t.Run("reads configured environment variable", func(t *testing.T) {
		t.Setenv("TEST_MODEL_API_KEY", "test-key")

		got, err := apiKey(config.ModelConfig{
			APIKeyEnv: "TEST_MODEL_API_KEY",
		})
		if err != nil {
			t.Fatalf("apiKey() error = %v", err)
		}

		if got != "test-key" {
			t.Fatalf("apiKey() = %q, want %q", got, "test-key")
		}
	})

	t.Run("requires environment variable name", func(t *testing.T) {
		_, err := apiKey(config.ModelConfig{})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("requires environment variable value", func(t *testing.T) {
		t.Setenv("TEST_EMPTY_MODEL_API_KEY", "")

		_, err := apiKey(config.ModelConfig{
			APIKeyEnv: "TEST_EMPTY_MODEL_API_KEY",
		})
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "TEST_EMPTY_MODEL_API_KEY") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestManagerGetMissingProfile(t *testing.T) {
	manager := NewManager(nil)

	_, err := manager.Get(
		context.Background(),
		"missing",
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestManagerGetCachesProfile(t *testing.T) {
	t.Setenv("TEST_DEEPSEEK_API_KEY", "test-key")

	manager := NewManager(
		map[string]config.ModelConfig{
			"default": {
				Provider:  config.ProviderDeepSeek,
				Model:     "deepseek-chat",
				APIKeyEnv: "TEST_DEEPSEEK_API_KEY",
			},
		},
	)

	first, err := manager.Get(
		context.Background(),
		"default",
	)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if first == nil {
		t.Fatal("expected model")
	}

	second, err := manager.Get(
		context.Background(),
		"default",
	)
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}

	if first != second {
		t.Fatal("expected cached model instance")
	}
}

func TestNewUnsupportedProvider(t *testing.T) {
	_, err := New(
		context.Background(),
		config.ModelConfig{
			Provider: "unknown",
			Model:    "test",
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
}


func TestDeepSeekResponseFormatModes(t *testing.T) {
	jsonMode, err := deepSeekResponseFormat(
		deepSeekResponseJSON,
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(jsonMode) != "json_object" {
		t.Fatalf(
			"json response mode = %q",
			jsonMode,
		)
	}

	toolMode, err := deepSeekResponseFormat(
		deepSeekResponseTool,
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(toolMode) != "text" {
		t.Fatalf(
			"tool response mode = %q",
			toolMode,
		)
	}

	if _, err := deepSeekResponseFormat(
		deepSeekResponseMode("invalid"),
	); err == nil {
		t.Fatal(
			"expected invalid response mode error",
		)
	}
}

func TestManagerGetToolCallingCachesDedicatedProfile(
	t *testing.T,
) {
	t.Setenv(
		"TEST_DEEPSEEK_TOOL_API_KEY",
		"test-key",
	)

	manager := NewManager(
		map[string]config.ModelConfig{
			"event": {
				Provider: config.ProviderDeepSeek,
				Model: "deepseek-chat",
				APIKeyEnv:
					"TEST_DEEPSEEK_TOOL_API_KEY",
			},
		},
	)

	first, err := manager.GetToolCalling(
		context.Background(),
		"event",
	)
	if err != nil {
		t.Fatalf(
			"GetToolCalling() error = %v",
			err,
		)
	}
	second, err := manager.GetToolCalling(
		context.Background(),
		"event",
	)
	if err != nil {
		t.Fatalf(
			"second GetToolCalling() error = %v",
			err,
		)
	}
	if first != second {
		t.Fatal(
			"expected cached tool-calling model instance",
		)
	}

	jsonModel, err := manager.Get(
		context.Background(),
		"event",
	)
	if err != nil {
		t.Fatalf(
			"Get() error = %v",
			err,
		)
	}
	if jsonModel == nil {
		t.Fatal("expected JSON model")
	}
	if len(manager.models) != 1 ||
		len(manager.toolCallingModels) != 1 {
		t.Fatalf(
			"manager caches json=%d tool=%d",
			len(manager.models),
			len(manager.toolCallingModels),
		)
	}
}

func TestDeepSeekRejectsInvalidMaxTokens(t *testing.T) {
	t.Setenv("TEST_MODEL_MAX_TOKENS_KEY", "test-key")
	for _, n := range []int{0, -1} {
		_, err := newDeepSeekForMode(context.Background(), config.ModelConfig{
			Provider:  config.ProviderDeepSeek,
			Model:     "deepseek-chat",
			APIKeyEnv: "TEST_MODEL_MAX_TOKENS_KEY",
			MaxTokens: &n,
		}, deepSeekResponseJSON)
		if err == nil || !strings.Contains(err.Error(), "max_tokens must be positive") {
			t.Fatalf("max_tokens=%d, error=%v", n, err)
		}
	}
}
