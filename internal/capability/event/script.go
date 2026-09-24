package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type ScriptSourceConfig struct {
	ID         string
	Name       string
	Kind       SourceKind
	Python     string
	ScriptPath string
}

type ScriptSource struct {
	id         string
	name       string
	kind       SourceKind
	python     string
	scriptPath string
}

func NewScriptSource(
	cfg ScriptSourceConfig,
) (*ScriptSource, error) {
	id := strings.TrimSpace(cfg.ID)
	if id == "" {
		return nil, errors.New(
			"event script source id is required",
		)
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return nil, errors.New(
			"event script source name is required",
		)
	}

	if cfg.Kind == "" {
		return nil, errors.New(
			"event script source kind is required",
		)
	}

	python := strings.TrimSpace(cfg.Python)
	if python == "" {
		python = "python3"
	}

	scriptPath := strings.TrimSpace(cfg.ScriptPath)
	if scriptPath == "" {
		return nil, errors.New(
			"event source script path is required",
		)
	}

	absolutePath, err := filepath.Abs(scriptPath)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve event source script path: %w",
			err,
		)
	}

	info, err := os.Stat(absolutePath)
	if err != nil {
		return nil, fmt.Errorf(
			"stat event source script %q: %w",
			absolutePath,
			err,
		)
	}

	if info.IsDir() {
		return nil, fmt.Errorf(
			"event source script path is a directory: %s",
			absolutePath,
		)
	}

	return &ScriptSource{
		id:         id,
		name:       name,
		kind:       cfg.Kind,
		python:     python,
		scriptPath: absolutePath,
	}, nil
}

func (s *ScriptSource) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *ScriptSource) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

func (s *ScriptSource) Kind() SourceKind {
	if s == nil {
		return ""
	}
	return s.kind
}

type scriptSourcePayload struct {
	EntityName string             `json:"entity_name"`
	Items      []scriptSourceItem `json:"items"`
}

type scriptSourceItem struct {
	PublishedAt string `json:"published_at"`
	Title       string `json:"title"`
	URL         string `json:"url,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
}

func (s *ScriptSource) Fetch(
	ctx context.Context,
	req Request,
) (SourceResult, error) {
	if s == nil {
		return SourceResult{}, errors.New(
			"event script source is nil",
		)
	}

	symbol := strings.TrimSpace(req.Symbol)
	if symbol == "" {
		return SourceResult{}, errors.New(
			"event source symbol is required",
		)
	}

	asOf := strings.TrimSpace(req.AsOf)
	if asOf == "" {
		return SourceResult{}, errors.New(
			"event source as_of is required",
		)
	}

	lookbackDays := req.LookbackDays
	if lookbackDays <= 0 {
		lookbackDays = defaultLookbackDays
	}

	cmd := exec.CommandContext(
		ctx,
		s.python,
		s.scriptPath,
		"--symbol",
		symbol,
		"--as-of",
		asOf,
		"--lookback-days",
		strconv.Itoa(lookbackDays),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return SourceResult{}, fmt.Errorf(
			"run event source script: %w: stderr=%s",
			err,
			strings.TrimSpace(stderr.String()),
		)
	}

	var payload scriptSourcePayload
	if err := json.Unmarshal(
		output,
		&payload,
	); err != nil {
		return SourceResult{}, fmt.Errorf(
			"decode event source script output: %w: output=%q",
			err,
			strings.TrimSpace(string(output)),
		)
	}

	items := make([]RawItem, 0, len(payload.Items))
	for _, item := range payload.Items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}

		items = append(
			items,
			RawItem{
				SourceID:    s.id,
				SourceName:  s.name,
				Kind:        s.kind,
				PublishedAt: strings.TrimSpace(item.PublishedAt),
				Title:       title,
				URL:         strings.TrimSpace(item.URL),
				Snippet:     strings.TrimSpace(item.Snippet),
			},
		)
	}

	status := CoverageCovered
	if len(items) == 0 {
		status = CoverageEmpty
	}

	return SourceResult{
		EntityName: strings.TrimSpace(
			payload.EntityName,
		),
		Coverage: Coverage{
			SourceID:   s.id,
			SourceName: s.name,
			Kind:       s.kind,
			Status:     status,
			ItemCount:  len(items),
		},
		Items: items,
	}, nil
}
