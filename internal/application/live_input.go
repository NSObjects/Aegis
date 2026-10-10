package application

import (
	"fmt"
	"strings"
	"time"

	eventagent "Aegis/internal/agent/event"
	eventcap "Aegis/internal/capability/event"
	"Aegis/internal/research"
)

// NewLiveResearchInput builds the original CLI live PIT-safe event task for
// any transport. It must use a caller-chosen cutoff; no hidden time.Now().
func NewLiveResearchInput(symbol, marketQuery string, cutoff time.Time, lookback time.Duration) (research.Input, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return research.Input{}, fmt.Errorf("research symbol is required")
	}
	if cutoff.IsZero() {
		return research.Input{}, fmt.Errorf("event publication cutoff is required")
	}
	if lookback <= 0 {
		return research.Input{}, fmt.Errorf("event lookback must be positive")
	}
	return research.Input{
		Symbol:        symbol,
		RequestedAsOf: cutoff,
		MarketQuery:   strings.TrimSpace(marketQuery),
		EventInput: eventagent.Input{
			Task: eventcap.ResearchTask{
				ID:              fmt.Sprintf("live-event-%s-%d", symbol, cutoff.UnixNano()),
				Symbol:          symbol,
				Objective:       "研究该标的近期公告、新闻与重要事件，并形成可追溯的事件上下文。",
				InformationNeed: eventcap.NeedEventEvidence,
				AsOf:            cutoff,
				TimeRange:       eventcap.TimeRange{Start: cutoff.Add(-lookback), End: cutoff},
				Requirements: eventcap.ResearchRequirements{
					PITSafe:               true,
					PreferredSourceTypes:  []eventcap.SourceType{eventcap.SourceOfficial, eventcap.SourceIndustryMedia, eventcap.SourceMainstreamMedia},
					MinDocuments:          1,
					MinIndependentSources: 1,
				},
			},
			Query: "研究请求时点之前的近期公司公告、新闻与重要事件。",
		},
	}, nil
}
