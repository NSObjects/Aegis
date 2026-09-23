package config

import "testing"

func TestModelProfileForAgent(t *testing.T) {
	cfg := Config{
		DefaultModel: "default",
		Models: map[string]ModelConfig{
			"default": {
				Provider: ProviderDeepSeek,
				Model:    "deepseek-chat",
				APIKey:   "DEEPSEEK_API_KEY",
			},
			"strong": {
				Provider: "openai",
				Model:    "gpt-5.6",
				APIKey:   "OPENAI_API_KEY",
			},
		},
		Agents: map[string]AgentConfig{
			"trading": {
				Model: "strong",
			},
		},
	}

	t.Run("use default model", func(t *testing.T) {
		profile, err := cfg.ModelProfileForAgent("market")
		if err != nil {
			t.Fatal(err)
		}

		if profile != "default" {
			t.Fatalf(
				"expected default, got %s",
				profile,
			)
		}
	})

	t.Run("agent overrides default model", func(t *testing.T) {
		profile, err := cfg.ModelProfileForAgent("trading")
		if err != nil {
			t.Fatal(err)
		}

		if profile != "strong" {
			t.Fatalf(
				"expected strong, got %s",
				profile,
			)
		}
	})

	t.Run("unknown override model", func(t *testing.T) {
		cfg.Agents["reflection"] = AgentConfig{
			Model: "missing",
		}

		_, err := cfg.ModelProfileForAgent("reflection")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing default model", func(t *testing.T) {
		bad := Config{
			Models: cfg.Models,
		}

		_, err := bad.ModelProfileForAgent("market")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
