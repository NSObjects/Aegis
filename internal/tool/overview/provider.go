package overview

import "context"

type Provider interface {
	Overview(ctx context.Context) (MarketOverview, error)
}

type StaticProvider struct {
	Data MarketOverview
}

func (p *StaticProvider) Overview(ctx context.Context) (MarketOverview, error) {
	return p.Data, nil
}
