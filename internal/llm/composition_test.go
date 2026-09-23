package llm

import (
	"Aegis/internal/config"
	"context"
	"fmt"
	"testing"
)

func TestModelComposition_DefaultAndAgentOverride(t *testing.T) {
	t.Setenv("TEST_DEEPSEEK_API_KEY", "deepseek-test-key")
	t.Setenv("TEST_OPENAI_API_KEY", "openai-test-key")

	cfg := config.Config{
		DefaultModel: "default",
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:  config.ProviderDeepSeek,
				Model:     "deepseek-chat",
				APIKeyEnv: "TEST_DEEPSEEK_API_KEY",
			},
			"strong": {
				Provider:  config.ProviderOpenAI,
				Model:     "test-openai-model",
				APIKeyEnv: "TEST_OPENAI_API_KEY",
			},
		},
		Agents: map[string]config.AgentConfig{
			"trading": {
				Model: "strong",
			},
		},
	}

	marketProfile, err := cfg.ModelProfileForAgent("market")
	if err != nil {
		t.Fatalf("resolve market profile: %v", err)
	}

	tradingProfile, err := cfg.ModelProfileForAgent("trading")
	if err != nil {
		t.Fatalf("resolve trading profile: %v", err)
	}

	if marketProfile != "default" {
		t.Fatalf(
			"market profile = %q, want %q",
			marketProfile,
			"default",
		)
	}

	if tradingProfile != "strong" {
		t.Fatalf(
			"trading profile = %q, want %q",
			tradingProfile,
			"strong",
		)
	}

	manager := NewManager(cfg.Models)

	marketModel, err := manager.Get(
		context.Background(),
		marketProfile,
	)
	if err != nil {
		t.Fatalf("create market model: %v", err)
	}

	tradingModel, err := manager.Get(
		context.Background(),
		tradingProfile,
	)
	if err != nil {
		t.Fatalf("create trading model: %v", err)
	}

	if marketModel == nil {
		t.Fatal("market model is nil")
	}

	if tradingModel == nil {
		t.Fatal("trading model is nil")
	}

	marketType := fmt.Sprintf("%T", marketModel)
	tradingType := fmt.Sprintf("%T", tradingModel)

	if marketType == tradingType {
		t.Fatalf(
			"expected different provider model types, both are %s",
			marketType,
		)
	}
}
