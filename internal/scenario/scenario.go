package scenario

import (
	"Aegis/internal/tool/market"
	"Aegis/internal/tool/news"
	"Aegis/internal/tool/technical"
)

type Scenario struct {
	Name      string
	Market    market.Snapshot
	Technical technical.Indicators
	News      []news.News
}
