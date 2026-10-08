package main

import (
	"Aegis/internal/portfolio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// loadRiskLimits reads a caller-controlled policy separately from the
// observed account snapshot. No defaults, invented financial limits, or
// implicit risk approval are introduced.
func loadRiskLimits(path string) (*portfolio.Limits, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("risk-limits-file path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read risk limits: %w", err)
	}
	var limits *portfolio.Limits
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&limits); err != nil {
		return nil, fmt.Errorf("decode risk limits: %w", err)
	}
	if limits == nil {
		return nil, errors.New("risk limits must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("risk limits must contain exactly one JSON object")
		}
		return nil, fmt.Errorf("decode trailing risk limits JSON: %w", err)
	}
	if err := portfolio.ValidateLimits(limits); err != nil {
		return nil, fmt.Errorf("invalid risk limits: %w", err)
	}
	return limits, nil
}
