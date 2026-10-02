package manager

import (
	"Aegis/internal/agent/event"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/research/synthesis"
)

func validManagerArtifact() *synthesis.Artifact {
	return &synthesis.Artifact{
		Symbol: "600519.SH",
		Market: &market.Context{
			AsOf: "20260930",
			MarketView: "neutral market",
			RiskLevel: "MEDIUM",
		},
		Technical: &technical.Context{
			Symbol: "600519.SH",
			RequestedAsOf: "20260930",
			DataAsOf: "20260930",
			Status: "NORMAL",
			Summary: "constructive technical context",
		},
		Event: &event.Context{
			Symbol: "600519.SH",
			AsOf: "20260930",
			Status: "NO_EVENT",
		},
		Synthesis: synthesis.Synthesis{
			Summary: "mixed evidence",
			Confidence: .6,
		},
	}
}
