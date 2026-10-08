package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Provider string

const (
	ProviderDeepSeek Provider = "deepseek"
)

type ModelConfig struct {
	Provider Provider `yaml:"provider"`
	Model    string   `yaml:"model"`

	APIKeyEnv string `yaml:"api_key_env"`
	BaseURL   string `yaml:"base_url,omitempty"`
	MaxTokens *int   `yaml:"max_tokens,omitempty"`
}

type AgentConfig struct {
	Model string `yaml:"model,omitempty"`
}

type Config struct {
	DefaultModel string `yaml:"default_model"`

	Models map[string]ModelConfig `yaml:"models"`
	Agents map[string]AgentConfig `yaml:"agents,omitempty"`
}

func Load(path string) (*Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New(
			"config path is required",
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf(
			"read config %q: %w",
			path,
			err,
		)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf(
			"decode config %q: %w",
			path,
			err,
		)
	}

	if strings.TrimSpace(cfg.DefaultModel) == "" {
		return nil, errors.New(
			"default_model is required",
		)
	}

	if len(cfg.Models) == 0 {
		return nil, errors.New(
			"models are required",
		)
	}

	if _, ok := cfg.Models[cfg.DefaultModel]; !ok {
		return nil, fmt.Errorf(
			"default model profile %q not found",
			cfg.DefaultModel,
		)
	}

	return &cfg, nil
}

func (c *Config) ModelProfileForAgent(agentName string) (string, error) {
	if c == nil {
		return "", errors.New(
			"config is nil",
		)
	}

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
