package portfolio

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

func Validate(ctx *Context) error {
	if ctx == nil {
		return errors.New("portfolio context is nil")
	}

	switch ctx.State {
	case StateNotProvided:
		if strings.TrimSpace(ctx.AsOf) != "" {
			return errors.New("as_of must be empty when portfolio is NOT_PROVIDED")
		}
		if len(ctx.Positions) != 0 {
			return errors.New("positions must be empty when portfolio is NOT_PROVIDED")
		}
		return nil

	case StateEmpty:
		if err := validateAsOf(ctx.AsOf); err != nil {
			return err
		}
		if len(ctx.Positions) != 0 {
			return errors.New("positions must be empty when portfolio is EMPTY")
		}
		return nil

	case StatePositions:
		if err := validateAsOf(ctx.AsOf); err != nil {
			return err
		}
		if len(ctx.Positions) == 0 {
			return errors.New("positions are required when portfolio state is POSITIONS")
		}

		seen := make(map[string]struct{}, len(ctx.Positions))
		for i, position := range ctx.Positions {
			symbol := strings.TrimSpace(position.Symbol)
			if symbol == "" {
				return fmt.Errorf("position %d symbol is required", i)
			}
			if symbol != strings.ToUpper(symbol) {
				return fmt.Errorf("position %d symbol %q must be uppercase", i, position.Symbol)
			}
			if math.IsNaN(position.Quantity) || math.IsInf(position.Quantity, 0) ||
				position.Quantity <= 0 {
				return fmt.Errorf("position %d quantity must be positive", i)
			}
			if _, ok := seen[symbol]; ok {
				return fmt.Errorf("position %d symbol %q is duplicated", i, symbol)
			}
			seen[symbol] = struct{}{}
		}
		return nil

	default:
		return fmt.Errorf("invalid portfolio state: %q", ctx.State)
	}
}

func validateAsOf(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return errors.New("as_of is required when portfolio is provided")
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("as_of must be RFC3339: %w", err)
	}
	return nil
}
