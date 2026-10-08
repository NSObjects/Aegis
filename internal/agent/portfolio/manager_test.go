package portfolioagent

import (
	"Aegis/internal/portfolio"
	"Aegis/internal/portfoliodecision"
	"Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"context"
	"errors"
	"testing"
)

func TestManagerReview(t *testing.T) {
	m := &Manager{}
	intent := &tradeintent.Intent{
		Symbol: "600519.SH",
		Thesis: "research thesis",
		Evidence: []string{"verified evidence"},
		Status: tradeintent.StatusCandidate,
		Confidence: 0.65,
	}
	book := &portfolio.Context{State: portfolio.StateEmpty, AsOf: "2026-10-08T09:00:00+08:00"}
	review := &risk.Context{Symbol: "600519.SH", Assessment: risk.AssessmentAcceptable, Summary: "no established conflict", Confidence: 0.7}
	result, err := m.Review(context.Background(), intent, book, review)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != portfoliodecision.DispositionReviewRequired {
		t.Fatalf("disposition = %q", result.Disposition)
	}
}

func TestManagerReviewContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&Manager{}).Review(ctx, nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestManagerReviewNilReceiver(t *testing.T) {
	var m *Manager
	_, err := m.Review(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("nil manager should fail")
	}
}
