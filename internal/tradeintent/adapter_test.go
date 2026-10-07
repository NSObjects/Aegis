package tradeintent

import (
	"Aegis/internal/decision"
	"strings"
	"testing"
)

func validDecision(action decision.Action) *decision.Decision {
	return &decision.Decision{
		Symbol:     "600519.SH",
		MarketView: "research thesis",
		Evidence:   []string{"grounded evidence"},
		Risk:       []string{"known risk"},
		Action:     action,
		Confidence: 0.68,
	}
}

func TestFromDecisionMapsActionsDeterministically(t *testing.T) {
	tests := []struct {
		action decision.Action
		want   Status
	}{
		{decision.ActionWatch, StatusMonitor},
		{decision.ActionAvoid, StatusRejected},
		{decision.ActionCandidate, StatusCandidate},
	}
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			intent, err := FromDecision(validDecision(tt.action))
			if err != nil {
				t.Fatalf("FromDecision() error = %v", err)
			}
			if intent.Status != tt.want {
				t.Fatalf("status = %q, want %q", intent.Status, tt.want)
			}
			if intent.Symbol != "600519.SH" || intent.Thesis != "research thesis" {
				t.Fatalf("intent = %+v", intent)
			}
		})
	}
}

func TestFromDecisionCopiesSlices(t *testing.T) {
	d := validDecision(decision.ActionCandidate)
	intent, err := FromDecision(d)
	if err != nil {
		t.Fatal(err)
	}
	intent.Evidence[0] = "mutated"
	intent.Risk[0] = "mutated"
	if d.Evidence[0] != "grounded evidence" || d.Risk[0] != "known risk" {
		t.Fatal("intent must not alias decision slices")
	}
}

func TestFromDecisionRejectsInvalidDecision(t *testing.T) {
	_, err := FromDecision(&decision.Decision{})
	if err == nil || !strings.Contains(err.Error(), "invalid trading decision") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInvalidIntent(t *testing.T) {
	tests := []struct {
		name string
		intent *Intent
		want string
	}{
		{"nil", nil, "nil"},
		{"symbol", &Intent{Thesis:"x", Evidence:[]string{"e"}, Status:StatusMonitor}, "symbol"},
		{"thesis", &Intent{Symbol:"600519.SH", Evidence:[]string{"e"}, Status:StatusMonitor}, "thesis"},
		{"status", &Intent{Symbol:"600519.SH", Thesis:"x", Evidence:[]string{"e"}, Status:"UNKNOWN"}, "status"},
		{"confidence", &Intent{Symbol:"600519.SH", Thesis:"x", Evidence:[]string{"e"}, Status:StatusMonitor, Confidence:1.1}, "confidence"},
		{"evidence", &Intent{Symbol:"600519.SH", Thesis:"x", Status:StatusMonitor}, "evidence"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.intent)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want contains %q", err, tt.want)
			}
		})
	}
}
