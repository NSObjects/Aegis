package runtime

import (
	"context"
	"errors"
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

func New(ctx context.Context, cfg Config) (*Runtime, error) {
	skillsDir := strings.TrimSpace(cfg.SkillsDir)
	if skillsDir == "" {
		return nil, errors.New("skills dir is required")
	}

	skillsDir, err := filepath.Abs(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path of skills dir: %w", err)
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
		return nil, fmt.Errorf("skills dir %q is not a directory", skillsDir)
	}

	location := strings.TrimSpace(cfg.TimeZone)
	if location == "" {
		return nil, errors.New("time zone is required")
	}

	timeZone, err := time.LoadLocation(location)
	if err != nil {
		return nil, fmt.Errorf("failed to load time zone: %w", err)
	}

	backend, err := local.NewBackend(ctx, &local.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create backend: %w", err)
	}
	
}
