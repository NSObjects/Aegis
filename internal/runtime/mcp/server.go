package mcp

import (
	"context"
	"fmt"
	"strings"

	officialmcp "github.com/cloudwego/eino-ext/components/tool/mcp/officialmcp"
	mcpsession "github.com/cloudwego/eino-ext/components/tool/mcp/officialmcp/session"
	"github.com/cloudwego/eino/components/tool"
)

type Server struct {
	name    string
	session *mcpsession.Session
	tools   []tool.BaseTool
}

func Connect(ctx context.Context, cfg ServerConfig) (*Server, error) {
	session, err := mcpsession.Connect(ctx, mcpsession.ServerConfig{
		Name: cfg.Name,
		Transport: mcpsession.TransportConfig{
			Type:    cfg.Transport,
			URL:     cfg.URL,
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     cfg.Env,
		},
		Client:            nil,
		ClientOptions:     nil,
		InitializeOptions: nil,
	})

	if err != nil {
		return nil, fmt.Errorf(
			"connect mcp server %q: %w",
			cfg.Name,
			err,
		)
	}
	tools, err := officialmcp.GetTools(ctx, &officialmcp.Config{
		Cli:           session,
		ServerName:    cfg.Name,
		ListToolsMode: officialmcp.ListToolsAllPages,
		ToolNameMapper: func(ctx context.Context, info officialmcp.ToolNameMapperInput) (officialmcp.ToolNameMapperOutput, error) {
			return officialmcp.ToolNameMapperOutput{
				ExposedName: normalizeToolName(info.ServerName + "_" + info.Tool.Name),
			}, nil
		},
	})

	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf(
			"get tools from mcp server %q: %w",
			cfg.Name,
			err,
		)
	}

	return &Server{
		name:    cfg.Name,
		session: session,
		tools:   tools,
	}, nil
}

func (s *Server) Tools() []tool.BaseTool {
	return s.tools
}

func (s *Server) Close() error {
	return s.session.Close()
}

func normalizeToolName(name string) string {
	replacer := strings.NewReplacer(
		"-", "_",
		".", "_",
		":", "_",
		"/", "_",
	)

	return replacer.Replace(name)
}
