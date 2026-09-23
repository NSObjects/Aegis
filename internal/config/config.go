package config

import (
	"errors"
	"fmt"
)

type Provider string

const (
	ProviderDeepSeek Provider = "deepseek"
	ProviderOpenAI   Provider = "openai"
	ProviderClaude   Provider = "claude"
	ProviderQwen     Provider = "qwen"
	ProviderGemini   Provider = "gemini"
	ProviderArk      Provider = "ark"
)

type ModelConfig struct {
	Provider Provider `yaml:"provider"`
	Model    string   `yaml:"model"`

	APIKeyEnv string `yaml:"api_key_env"`
	BaseURL   string `yaml:"base_url,omitempty"`

	MaxTokens *int `yaml:"max_tokens,omitempty"`
}

type AgentConfig struct {
	Model string `yaml:"model,omitempty"`
}

type Config struct {
	DefaultModel string `yaml:"default_model"`

	Models map[string]ModelConfig `yaml:"models"`
	Agents map[string]AgentConfig `yaml:"agents,omitempty"`
}

func (c *Config) ModelProfileForAgent(agentName string) (string, error) {
	profile := c.DefaultModel

	if agent, ok := c.Agents[agentName]; ok && agent.Model != "" {
		profile = agent.Model
	}

	if profile == "" {
		return "", errors.New("default_model is required")
	}

	if _, ok := c.Models[profile]; !ok {
		return "", fmt.Errorf(
			"model profile %q not found for agent %q",
			profile,
			agentName,
		)
	}

	return profile, nil
}
