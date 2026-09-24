package event

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const defaultLookbackDays = 7

type Request struct {
	Symbol       string `json:"symbol" jsonschema:"required" jsonschema_description:"A-share symbol, for example 600519.SH"`
	AsOf         string `json:"as_of" jsonschema:"required" jsonschema_description:"Research date in YYYYMMDD format"`
	LookbackDays int    `json:"lookback_days" jsonschema_description:"Calendar-day lookback window ending at as_of; defaults to 7"`
}

type Result struct {
	Symbol        string     `json:"symbol"`
	RequestedAsOf string     `json:"requested_as_of"`
	WindowStart   string     `json:"window_start"`
	WindowEnd     string     `json:"window_end"`
	EntityName    string     `json:"entity_name,omitempty"`
	Coverage      []Coverage `json:"coverage"`
	Items         []RawItem  `json:"items"`
}

type Researcher interface {
	Research(
		ctx context.Context,
		req Request,
	) (*Result, error)
}

type MultiSourceResearcher struct {
	sources []Source
}

func NewMultiSourceResearcher(
	sources ...Source,
) (*MultiSourceResearcher, error) {
	if len(sources) == 0 {
		return nil, errors.New(
			"at least one event source is required",
		)
	}

	seen := make(map[string]struct{}, len(sources))

	for i, source := range sources {
		if source == nil {
			return nil, fmt.Errorf(
				"event source[%d] is nil",
				i,
			)
		}

		id := strings.TrimSpace(source.ID())
		if id == "" {
			return nil, fmt.Errorf(
				"event source[%d] id is required",
				i,
			)
		}

		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf(
				"duplicate event source id: %s",
				id,
			)
		}
		seen[id] = struct{}{}
	}

	return &MultiSourceResearcher{
		sources: append([]Source(nil), sources...),
	}, nil
}

func (r *MultiSourceResearcher) Research(
	ctx context.Context,
	req Request,
) (*Result, error) {
	if r == nil {
		return nil, errors.New(
			"event researcher is nil",
		)
	}

	symbol := strings.TrimSpace(req.Symbol)
	if symbol == "" {
		return nil, errors.New(
			"event research symbol is required",
		)
	}

	asOf := strings.TrimSpace(req.AsOf)
	if asOf == "" {
		return nil, errors.New(
			"event research as_of is required",
		)
	}

	asOfTime, err := time.Parse("20060102", asOf)
	if err != nil {
		return nil, fmt.Errorf(
			"parse event research as_of: %w",
			err,
		)
	}

	lookbackDays := req.LookbackDays
	if lookbackDays <= 0 {
		lookbackDays = defaultLookbackDays
	}

	windowStart := asOfTime.AddDate(
		0,
		0,
		-(lookbackDays - 1),
	).Format("20060102")

	result := &Result{
		Symbol:        symbol,
		RequestedAsOf: asOf,
		WindowStart:   windowStart,
		WindowEnd:     asOf,
		Coverage:      make([]Coverage, 0, len(r.sources)),
		Items:         make([]RawItem, 0),
	}

	for _, source := range r.sources {
		sourceResult, fetchErr := source.Fetch(
			ctx,
			Request{
				Symbol:       symbol,
				AsOf:         asOf,
				LookbackDays: lookbackDays,
			},
		)

		if fetchErr != nil {
			result.Coverage = append(
				result.Coverage,
				Coverage{
					SourceID:   source.ID(),
					SourceName: source.Name(),
					Kind:       source.Kind(),
					Status:     CoverageFailed,
					Note:       fetchErr.Error(),
				},
			)
			continue
		}

		if result.EntityName == "" {
			result.EntityName = strings.TrimSpace(
				sourceResult.EntityName,
			)
		}

		result.Coverage = append(
			result.Coverage,
			sourceResult.Coverage,
		)
		result.Items = append(
			result.Items,
			sourceResult.Items...,
		)
	}

	return result, nil
}
