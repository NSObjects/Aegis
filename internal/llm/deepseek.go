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

func newDeepSeek(
	ctx context.Context,
	cfg config.ModelConfig,
) (
	model.BaseModel[*schema.AgenticMessage],
	error,
) {
	apiKey, err := apiKey(cfg)
	if err != nil {
		return nil, err
	}

	return agenticdeepseek.New(
		ctx,
		&agenticdeepseek.Config{
			APIKey:  apiKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,

			ResponseFormatType: agenticdeepseek.ResponseFormatTypeJSONObject,
		},
	)
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
