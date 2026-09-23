package llm

import (
	"Aegis/internal/config"
	"context"

	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func newOpenAI(
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

	return agenticopenai.NewChatModel(
		ctx,
		&agenticopenai.ChatConfig{
			APIKey:              apiKey,
			BaseURL:             cfg.BaseURL,
			Model:               cfg.Model,
			MaxCompletionTokens: cfg.MaxTokens,
		},
	)
}
