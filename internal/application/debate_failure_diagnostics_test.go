package application

import (
	"errors"
	"testing"
)

func TestDebateFailureCategories(t *testing.T) {
	cases := []struct{ message, want string }{
		{
			"research debate: bull round 2: generate bull researcher turn: 429 rate limit exceeded",
			"DEBATE_MODEL_GENERATE",
		},
		{
			"research debate: bear round 1: generate bear researcher turn: connection reset",
			"DEBATE_MODEL_GENERATE",
		},
		{
			"research debate: bull round 2: invalid bull researcher turn after 2 attempts: invalid bull researcher turn: argument cites unknown reference",
			"DEBATE_TURN_VALIDATION",
		},
		{
			"research debate: bear round 2: bear turn must rebut at least one prior bull argument",
			"DEBATE_TURN_VALIDATION",
		},
		{
			"research debate: invalid debate state: turn round mismatch",
			"DEBATE_RUN",
		},
		{
			"research debate returned no turns",
			"DEBATE_RUN",
		},
		{
			"invalid debate state: duplicate argument id",
			"DEBATE_RUN",
		},
	}
	for _, tc := range cases {
		t.Run(tc.want+"_"+tc.message, func(t *testing.T) {
			if got := researchErrorCategory(errors.New(tc.message)); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
