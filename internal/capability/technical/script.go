package technical

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ScriptResearcherConfig struct {
	Python     string
	ScriptPath string
}

type ScriptResearcher struct {
	python     string
	scriptPath string
}

func NewScriptResearcher(
	cfg ScriptResearcherConfig,
) (*ScriptResearcher, error) {
	python := strings.TrimSpace(cfg.Python)
	if python == "" {
		python = "python3"
	}

	scriptPath := strings.TrimSpace(cfg.ScriptPath)
	if scriptPath == "" {
		return nil, errors.New(
			"technical research script path is required",
		)
	}

	absolutePath, err := filepath.Abs(scriptPath)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve technical research script path: %w",
			err,
		)
	}

	info, err := os.Stat(absolutePath)
	if err != nil {
		return nil, fmt.Errorf(
			"stat technical research script %q: %w",
			absolutePath,
			err,
		)
	}

	if info.IsDir() {
		return nil, fmt.Errorf(
			"technical research script path is a directory: %s",
			absolutePath,
		)
	}

	return &ScriptResearcher{
		python:     python,
		scriptPath: absolutePath,
	}, nil
}

func (r *ScriptResearcher) Research(
	ctx context.Context,
	req Request,
) (map[string]any, error) {
	if r == nil {
		return nil, errors.New(
			"technical script researcher is nil",
		)
	}

	symbol := strings.TrimSpace(req.Symbol)
	if symbol == "" {
		return nil, errors.New(
			"technical research symbol is required",
		)
	}

	asOf := strings.TrimSpace(req.AsOf)
	if asOf == "" {
		return nil, errors.New(
			"technical research as_of is required",
		)
	}

	cmd := exec.CommandContext(
		ctx,
		r.python,
		r.scriptPath,
		"--symbol",
		symbol,
		"--as-of",
		asOf,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf(
			"run technical research script: %w: stderr=%s",
			err,
			strings.TrimSpace(stderr.String()),
		)
	}

	var result map[string]any
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf(
			"decode technical research script output: %w: output=%q",
			err,
			strings.TrimSpace(string(output)),
		)
	}

	return result, nil
}
