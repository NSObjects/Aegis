package application

import (
 "context"
 "strings"
 "testing"
 "time"
 "Aegis/internal/portfolio"
)
func TestLiveServiceFactoryFailsBeforeLoadingProviders(t *testing.T) {
 cases := []struct{name string; ctx context.Context; config LiveServiceOptions; want string}{
  {"nil context", nil, LiveServiceOptions{}, "context is nil"},
  {"invalid lookback", context.Background(), LiveServiceOptions{}, "lookback must be positive"},
  {"negative max age", context.Background(), LiveServiceOptions{EventLookback:time.Hour,PortfolioMaxAge:-time.Hour}, "max age"},
  {"portfolio policy without portfolio mode", context.Background(), LiveServiceOptions{EventLookback:time.Hour,PortfolioMaxAge:time.Hour}, "requires portfolio"},
  {"scenario without limits",context.Background(),LiveServiceOptions{EventLookback:time.Hour,PortfolioReview:true,Scenario:&portfolio.TradeScenario{}}, "requires current risk limits"},
 }
 canceled,cancel:=context.WithCancel(context.Background());cancel()
 cases=append(cases,struct{name string; ctx context.Context; config LiveServiceOptions; want string}{"canceled context",canceled,LiveServiceOptions{EventLookback:time.Hour},"canceled"})
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){svc,err:=NewLiveResearchService(tc.ctx,tc.config);if svc!=nil ||err==nil || !strings.Contains(err.Error(),tc.want){t.Fatalf("got service %v, err %v; want %q",svc,err,tc.want)}})}
}
