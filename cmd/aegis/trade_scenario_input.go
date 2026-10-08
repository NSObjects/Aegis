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

// loadTradeScenario accepts a caller-authored monetary what-if, not an order.
// Nested unknown fields are rejected; omitted/null notional is rejected by
// the domain validator instead of being silently interpreted as zero.
func loadTradeScenario(path string) (*portfolio.TradeScenario, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("trade-scenario-file path is required")
	}
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return nil, fmt.Errorf("read trade scenario: %w", err)
	}
	var scenario *portfolio.TradeScenario
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&scenario); err != nil {
		return nil, fmt.Errorf("decode trade scenario: %w", err)
	}
	if scenario == nil {
		return nil, errors.New("trade scenario must be a JSON object")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("trade scenario must contain exactly one JSON object")
		}
		return nil, fmt.Errorf("decode trade scenario trailing JSON: %w", err)
	}
	return scenario, nil
}
