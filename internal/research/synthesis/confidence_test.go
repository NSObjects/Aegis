package synthesis

import (
	"math"
	"strings"
	"testing"
)

func TestValidateSynthesisRejectsNonFiniteConfidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value float64
		valid bool
	}{
		{name: "zero", value: 0, valid: true},
		{name: "one", value: 1, valid: true},
		{name: "nan", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
		{name: "below zero", value: -0.01},
		{name: "above one", value: 1.01},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSynthesis(Synthesis{Summary: "grounded", Confidence: tc.value}, validSynthesisInput().Event)
			if tc.valid && err != nil {
				t.Fatalf("valid confidence %v rejected: %v", tc.value, err)
			}
			if !tc.valid && (err == nil || !strings.Contains(err.Error(), "confidence out of range")) {
				t.Fatalf("invalid confidence %v: got %v, want range error", tc.value, err)
			}
		})
	}
}
