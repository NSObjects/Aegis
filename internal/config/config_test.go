package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_ModelProfileForAgent(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		agentName string
		want      string
		wantErr   bool
	}{
		{
			name: "uses default when agent has no config",
			cfg: Config{
				DefaultModel: "default",
				Models: map[string]ModelConfig{
					"default": {
						Provider:  ProviderDeepSeek,
						Model:     "deepseek-chat",
						APIKeyEnv: "DEEPSEEK_API_KEY",
					},
				},
			},
			agentName: "market",
			want:      "default",
		},
		{
			name: "uses default when agent has empty override",
			cfg: Config{
				DefaultModel: "default",
				Models: map[string]ModelConfig{
					"default": {
						Provider:  ProviderDeepSeek,
						Model:     "deepseek-chat",
						APIKeyEnv: "DEEPSEEK_API_KEY",
					},
				},
				Agents: map[string]AgentConfig{
					"market": {},
				},
			},
			agentName: "market",
			want:      "default",
		},
		{
			name: "agent override wins",
			cfg: Config{
				DefaultModel: "default",
				Models: map[string]ModelConfig{
					"default": {
						Provider:  ProviderDeepSeek,
						Model:     "deepseek-chat",
						APIKeyEnv: "DEEPSEEK_API_KEY",
					},
					"strong": {
						Provider:  ProviderDeepSeek,
						Model:     "deepseek-reasoner",
						APIKeyEnv: "DEEPSEEK_API_KEY",
					},
				},
				Agents: map[string]AgentConfig{
					"trading": {
						Model: "strong",
					},
				},
			},
			agentName: "trading",
			want:      "strong",
		},
		{
			name: "unknown override profile",
			cfg: Config{
				DefaultModel: "default",
				Models: map[string]ModelConfig{
					"default": {
						Provider:  ProviderDeepSeek,
						Model:     "deepseek-chat",
						APIKeyEnv: "DEEPSEEK_API_KEY",
					},
				},
				Agents: map[string]AgentConfig{
					"trading": {
						Model: "missing",
					},
				},
			},
			agentName: "trading",
			wantErr:   true,
		},
		{
			name: "missing default model",
			cfg: Config{
				Models: map[string]ModelConfig{
					"default": {
						Provider:  ProviderDeepSeek,
						Model:     "deepseek-chat",
						APIKeyEnv: "DEEPSEEK_API_KEY",
					},
				},
			},
			agentName: "market",
			wantErr:   true,
		},
		{
			name: "unknown default profile",
			cfg: Config{
				DefaultModel: "missing",
				Models: map[string]ModelConfig{
					"default": {
						Provider:  ProviderDeepSeek,
						Model:     "deepseek-chat",
						APIKeyEnv: "DEEPSEEK_API_KEY",
					},
				},
			},
			agentName: "market",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cfg.ModelProfileForAgent(tt.agentName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("ModelProfileForAgent() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf(
					"ModelProfileForAgent() = %q, want %q",
					got,
					tt.want,
				)
			}
		})
	}
}


func TestLoad(t *testing.T) {
	t.Run("loads yaml", func(t *testing.T) {
		path := writeTestConfig(
			t,
			`default_model: default
models:
  default:
    provider: deepseek
    model: deepseek-chat
    api_key_env: DEEPSEEK_API_KEY
agents:
  trading:
    model: default
`,
		)

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.DefaultModel != "default" {
			t.Fatalf(
				"DefaultModel = %q",
				cfg.DefaultModel,
			)
		}

		if cfg.Models["default"].Provider !=
			ProviderDeepSeek {
			t.Fatalf(
				"provider = %q",
				cfg.Models["default"].Provider,
			)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := Load(
			filepath.Join(
				t.TempDir(),
				"missing.yaml",
			),
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := writeTestConfig(
			t,
			"default_model: [",
		)

		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing default model", func(t *testing.T) {
		path := writeTestConfig(
			t,
			`models:
  default:
    provider: deepseek
    model: deepseek-chat
    api_key_env: DEEPSEEK_API_KEY
`,
		)

		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unknown default profile", func(t *testing.T) {
		path := writeTestConfig(
			t,
			`default_model: missing
models:
  default:
    provider: deepseek
    model: deepseek-chat
    api_key_env: DEEPSEEK_API_KEY
`,
		)

		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestConfig_ModelProfileForAgent_NilConfig(
	t *testing.T,
) {
	var cfg *Config

	_, err := cfg.ModelProfileForAgent(
		"market",
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func writeTestConfig(
	t *testing.T,
	content string,
) string {
	t.Helper()

	path := filepath.Join(
		t.TempDir(),
		"config.yaml",
	)

	if err := os.WriteFile(
		path,
		[]byte(content),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestLoadModelMaxTokens(t *testing.T) {
	path := writeTestConfig(t, `default_model: default
models:
  default:
    provider: deepseek
    model: deepseek-chat
    api_key_env: DEEPSEEK_API_KEY
    max_tokens: 16384
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	tokens := cfg.Models["default"].MaxTokens
	if tokens == nil || *tokens != 16384 {
		t.Fatalf("max_tokens = %v, want 16384", tokens)
	}
}
