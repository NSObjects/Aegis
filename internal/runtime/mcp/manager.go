package mcp

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/components/tool"
)

type Manager struct {
	servers  []*Server
	tools    []tool.BaseTool
	failures map[string]error
}

func ConnectAll(ctx context.Context, configs []ServerConfig) (*Manager, error) {
	m := &Manager{
		failures: make(map[string]error),
	}
	for _, cfg := range configs {
		server, err := Connect(ctx, cfg)
		if err != nil {
			m.failures[cfg.Name] = err
			continue
		}

		m.servers = append(m.servers, server)
		m.tools = append(m.tools, server.Tools()...)
	}

	return m, nil
}

func (m *Manager) Tools() []tool.BaseTool {
	return m.tools
}

func (m *Manager) Failures() map[string]error {
	return m.failures
}

func (m *Manager) Close() error {
	var errs []error

	for _, server := range m.servers {
		if err := server.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
