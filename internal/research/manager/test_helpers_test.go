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
			AsOf:      "20260930",
			Regime:    market.RegimeMixed,
			Evidence:  []string{"market evidence"},
			Confidence: .6,
		},
		Technical: &technical.Context{
			Symbol:        "600519.SH",
			RequestedAsOf: "20260930",
			EffectiveAsOf: "20260930",
			Status:        technical.StatusNormal,
			Snapshot: &technical.Snapshot{
				Open: 1, High: 1, Low: 1, Close: 1, PreClose: 1,
			},
			Evidence:   []string{"technical evidence"},
			Confidence: .7,
		},
		Event: &event.Context{
			Symbol:        "600519.SH",
			RequestedAsOf: "2026-09-30T15:00:00+08:00",
			Status:        event.StatusNoEvent,
			Confidence:    .5,
		},
		Synthesis: synthesis.Synthesis{
			Summary:    "mixed evidence",
			Confidence: .6,
		},
	}
}
