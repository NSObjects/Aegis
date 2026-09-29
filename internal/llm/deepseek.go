package llm

import (
	"Aegis/internal/config"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/agenticdeepseek"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type deepSeekResponseMode string

const (
	deepSeekResponseJSON deepSeekResponseMode = "json"
	deepSeekResponseTool deepSeekResponseMode = "tool"
)

func newDeepSeek(
	ctx context.Context,
	cfg config.ModelConfig,
) (
	model.BaseModel[*schema.AgenticMessage],
	error,
) {
	return newDeepSeekForMode(
		ctx,
		cfg,
		deepSeekResponseJSON,
	)
}

func newDeepSeekToolCalling(
	ctx context.Context,
	cfg config.ModelConfig,
) (
	model.BaseModel[*schema.AgenticMessage],
	error,
) {
	return newDeepSeekForMode(
		ctx,
		cfg,
		deepSeekResponseTool,
	)
}

func newDeepSeekForMode(
	ctx context.Context,
	cfg config.ModelConfig,
	mode deepSeekResponseMode,
) (
	model.BaseModel[*schema.AgenticMessage],
	error,
) {
	apiKey, err := apiKey(cfg)
	if err != nil {
		return nil, err
	}

	responseFormat, err := deepSeekResponseFormat(
		mode,
	)
	if err != nil {
		return nil, err
	}

	base, err := agenticdeepseek.New(
		ctx,
		&agenticdeepseek.Config{
			APIKey:  apiKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,

			ResponseFormatType: responseFormat,
		},
	)
	if err != nil {
		return nil, err
	}

	return newDSMLNormalizingModel(base), nil
}

func deepSeekResponseFormat(
	mode deepSeekResponseMode,
) (
	agenticdeepseek.ResponseFormatType,
	error,
) {
	switch mode {
	case deepSeekResponseJSON:
		return agenticdeepseek.
			ResponseFormatTypeJSONObject, nil
	case deepSeekResponseTool:
		return agenticdeepseek.
			ResponseFormatTypeText, nil
	default:
		return "", fmt.Errorf(
			"unsupported DeepSeek response mode %q",
			mode,
		)
	}
}

func apiKey(
	cfg config.ModelConfig,
) (string, error) {
	if cfg.APIKeyEnv == "" {
		return "", errors.New(
			"api_key_env is required",
		)
	}

	value := strings.TrimSpace(
		os.Getenv(cfg.APIKeyEnv),
	)

	if value == "" {
		return "", fmt.Errorf(
			"environment variable %s is not set",
			cfg.APIKeyEnv,
		)
	}

	return value, nil
}
