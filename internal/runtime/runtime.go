package runtime

import (
	"Aegis/internal/middleware"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type Config struct {
	SkillsDir string
	TimeZone  string
}

type Runtime struct {
	handlers []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]
}

func New(
	ctx context.Context,
	cfg Config,
) (*Runtime, error) {
	skillsDir := strings.TrimSpace(cfg.SkillsDir)
	if skillsDir == "" {
		return nil, fmt.Errorf("runtime skills directory is required")
	}

	skillsDir, err := filepath.Abs(skillsDir)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve runtime skills directory: %w",
			err,
		)
	}

	info, err := os.Stat(skillsDir)
	if err != nil {
		return nil, fmt.Errorf(
			"stat runtime skills directory %q: %w",
			skillsDir,
			err,
		)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf(
			"runtime skills path is not a directory: %s",
			skillsDir,
		)
	}

	timeZone := strings.TrimSpace(cfg.TimeZone)
	if timeZone == "" {
		return nil, fmt.Errorf("runtime timezone is required")
	}

	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return nil, fmt.Errorf(
			"load runtime timezone %q: %w",
			timeZone,
			err,
		)
	}

	backend, err := local.NewBackend(
		ctx,
		&local.Config{},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create runtime local backend: %w",
			err,
		)
	}

	skillMW, err := middleware.NewSkill(
		ctx,
		backend,
		skillsDir,
	)
	if err != nil {
		return nil, err
	}

	filesystemMW, err := middleware.NewFilesystem(
		ctx,
		backend,
		backend,
	)
	if err != nil {
		return nil, err
	}

	contextMW := middleware.NewContext[*schema.AgenticMessage](
		location,
	)

	return &Runtime{
		handlers: []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{
			skillMW,
			filesystemMW,
			contextMW,
		},
	}, nil
}

func (r *Runtime) Handlers() []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage] {
	if r == nil {
		return nil
	}

	return append(
		[]adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage](nil),
		r.handlers...,
	)
}
