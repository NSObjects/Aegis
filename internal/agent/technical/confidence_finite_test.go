package technical

import (
	"math"
	"strings"
	"testing"
)

func TestValidateConfidenceFinite(t *testing.T) {
	tests := []struct {
		name string
		confidence float64
		valid bool
	}{
		{"zero", 0, true},
		{"one", 1, true},
		{"nan", math.NaN(), false},
		{"positive infinity", math.Inf(1), false},
		{"negative infinity", math.Inf(-1), false},
		{"below zero", -0.01, false},
		{"above one", 1.01, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &Context{
				Symbol: "600519.SH",
				RequestedAsOf: "20260923",
				Status: StatusNoData,
				Risk: []string{"no data available"},
				Confidence: tc.confidence,
			}
			err := Validate(ctx)
			if tc.valid && err != nil {
				t.Fatalf("valid confidence rejected: %v", err)
			}
			if !tc.valid && (err == nil || !strings.Contains(err.Error(), "confidence out of range")) {
				t.Fatalf("confidence %v unexpectedly accepted or wrong error: %v", tc.confidence, err)
			}
		})
	}
}
