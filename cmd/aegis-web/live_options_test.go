package main

import (
    "testing"
    "Aegis/internal/application"
)

// A Conversation DECISION request must not accidentally execute against the
// server's default Portfolio Review-only runner.
func TestLiveOptionsFollowPersistedRunMode(t *testing.T) {
    for _, tc := range []struct {
        name string
        serverPortfolio bool
        mode application.ResearchMode
        wantPortfolio bool
    }{
        {"conversation overrides portfolio default", true, application.ModeDecision, false},
        {"portfolio overrides decision default", false, application.ModePortfolio, true},
        {"decision default", false, application.ModeDecision, false},
        {"portfolio default", true, application.ModePortfolio, true},
    } {
        t.Run(tc.name, func(t *testing.T) {
            base := application.LiveServiceOptions{PortfolioReview: tc.serverPortfolio}
            opts := liveOptionsForRequest(base, application.ResearchRequest{Mode:tc.mode})
            if opts.PortfolioReview != tc.wantPortfolio {
                t.Fatalf("mode=%s default=%v got graph portfolio=%v, want=%v", tc.mode, tc.serverPortfolio, opts.PortfolioReview, tc.wantPortfolio)
            }
            if base.PortfolioReview != tc.serverPortfolio {
                t.Fatal("changed shared base configuration")
            }
        })
    }
}
