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

// loadPortfolioContext accepts only an explicitly supplied account snapshot.
// In particular, the absence of a file is NOT equivalent to EMPTY or a
// synthetic test portfolio. Use {"state":"NOT_PROVIDED"} when unknown.
func loadPortfolioContext(path string) (*portfolio.Context, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("portfolio-review requires -portfolio-file with explicit JSON context")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read portfolio context: %w", err)
	}
	var book portfolio.Context
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&book); err != nil {
		return nil, fmt.Errorf("decode portfolio context: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("portfolio context must contain exactly one JSON value")
		}
		return nil, fmt.Errorf("decode portfolio context trailing JSON: %w", err)
	}
	if err := portfolio.Validate(&book); err != nil {
		return nil, fmt.Errorf("invalid portfolio context: %w", err)
	}
	return &book, nil
}
