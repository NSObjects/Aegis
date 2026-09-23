package config

import "testing"

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
