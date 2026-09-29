package llm

import (
	"Aegis/internal/config"
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Manager struct {
	mu sync.Mutex

	configs map[string]config.ModelConfig
	models  map[string]model.BaseModel[*schema.AgenticMessage]
}

func NewManager(configs map[string]config.ModelConfig) *Manager {
	return &Manager{
		configs: configs,
		models:  make(map[string]model.BaseModel[*schema.AgenticMessage]),
	}
}

func (m *Manager) Get(
	ctx context.Context,
	profile string,
) (
	model.BaseModel[*schema.AgenticMessage],
	error,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.models[profile]; ok {
		return existing, nil
	}

	cfg, ok := m.configs[profile]
	if !ok {
		return nil, fmt.Errorf(
			"model profile %q not found",
			profile,
		)
	}

	instance, err := New(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf(
			"create model profile %q: %w",
			profile,
			err,
		)
	}

	m.models[profile] = instance

	return instance, nil
}

func (m *Manager) GetToolCalling(
	ctx context.Context,
	profile string,
) (
	model.ToolCallingChatModel,
	error,
) {
	base, err := m.Get(ctx, profile)
	if err != nil {
		return nil, err
	}

	adapter, err := NewAgenticToolCallingAdapter(
		base,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"adapt model profile %q for tool calling: %w",
			profile,
			err,
		)
	}
	return adapter, nil
}

func New(
	ctx context.Context,
	cfg config.ModelConfig,
) (
	model.BaseModel[*schema.AgenticMessage],
	error,
) {
	switch cfg.Provider {
	case config.ProviderDeepSeek:
		return newDeepSeek(ctx, cfg)

	default:
		return nil, fmt.Errorf(
			"unsupported provider: %q",
			cfg.Provider,
		)
	}
}
