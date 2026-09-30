package debate

import (
	"Aegis/internal/research/synthesis"
	"context"
	"errors"
	"fmt"
)

const DefaultMaxRounds = 2

type Researcher interface {
	GenerateTurn(ctx context.Context, artifact *synthesis.Artifact, state *State, round int) (Turn, error)
}

type Runtime struct {
	Bull      Researcher
	Bear      Researcher
	MaxRounds int
}

func (r *Runtime) Run(ctx context.Context, artifact *synthesis.Artifact) (*State, error) {
	if r == nil { return nil, errors.New("debate runtime is nil") }
	if r.Bull == nil { return nil, errors.New("bull researcher is required") }
	if r.Bear == nil { return nil, errors.New("bear researcher is required") }
	if err := synthesis.Validate(artifact); err != nil { return nil, fmt.Errorf("invalid research artifact: %w", err) }

	maxRounds := r.MaxRounds
	if maxRounds == 0 { maxRounds = DefaultMaxRounds }
	if maxRounds < 0 { return nil, errors.New("max debate rounds must not be negative") }

	state := &State{Symbol: artifact.Symbol}
	for round := 1; round <= maxRounds; round++ {
		if err := ctx.Err(); err != nil { return nil, err }

		bullTurn, err := r.Bull.GenerateTurn(ctx, artifact, cloneState(state), round)
		if err != nil { return nil, fmt.Errorf("bull round %d: %w", round, err) }
		if err := appendExpectedTurn(state, artifact, bullTurn, round, SideBull); err != nil {
			return nil, fmt.Errorf("bull round %d: %w", round, err)
		}

		if err := ctx.Err(); err != nil { return nil, err }
		bearTurn, err := r.Bear.GenerateTurn(ctx, artifact, cloneState(state), round)
		if err != nil { return nil, fmt.Errorf("bear round %d: %w", round, err) }
		if err := appendExpectedTurn(state, artifact, bearTurn, round, SideBear); err != nil {
			return nil, fmt.Errorf("bear round %d: %w", round, err)
		}
	}
	return cloneState(state), nil
}

func appendExpectedTurn(state *State, artifact *synthesis.Artifact, turn Turn, round int, side Side) error {
	if turn.Round != round { return fmt.Errorf("returned round %d, want %d", turn.Round, round) }
	if turn.Side != side { return fmt.Errorf("returned side %q, want %q", turn.Side, side) }
	state.Turns = append(state.Turns, turn)
	if err := ValidateState(state, artifact); err != nil {
		state.Turns = state.Turns[:len(state.Turns)-1]
		return fmt.Errorf("invalid debate state: %w", err)
	}
	return nil
}

func cloneState(state *State) *State {
	if state == nil { return nil }
	out := &State{Symbol: state.Symbol, Turns: make([]Turn, len(state.Turns))}
	for i, turn := range state.Turns {
		out.Turns[i] = turn
		out.Turns[i].Arguments = append([]Argument(nil), turn.Arguments...)
		out.Turns[i].Rebuttals = append([]Rebuttal(nil), turn.Rebuttals...)
	}
	return out
}
