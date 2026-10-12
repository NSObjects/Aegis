package research

import (
 "context"
 "errors"
 "strings"
 "testing"
 "time"

 "Aegis/internal/agent/market"
 eventagent "Aegis/internal/agent/event"
 "github.com/cloudwego/eino/adk"
)

type cancellationAwareMarket struct {
 started chan struct{}
 stopped chan struct{}
}
func (a *cancellationAwareMarket) Analyze(ctx context.Context, _ string, _ ...adk.AgentRunOption) (*market.Context,error) {
 close(a.started)
 <-ctx.Done()
 close(a.stopped)
 return nil,ctx.Err()
}

func TestEventFailureCancelsParallelMarket(t *testing.T) {
 marketAgent:=&cancellationAwareMarket{started:make(chan struct{}),stopped:make(chan struct{})}
 eventAgent:=&fakeEventAgent{err:errors.New("synthetic event validation error"),release:marketAgent.started}
 w,err:=New(context.Background(),Config{
  MarketAgent:marketAgent,TechnicalAgent:&fakeTechnicalAgent{result:validTechnicalContext()},
  EventAgent:eventAgent,Synthesizer:&fakeSynthesizer{},TradingAgent:&fakeArtifactTradingAgent{result:validDecision()},
 })
 if err!=nil {t.Fatal(err)}
 result:=make(chan error,1)
 go func(){_,err:=w.RunArtifact(context.Background(),validWorkflowInput());result<-err}()
 select {
 case err:=<-result:
  if err==nil || !strings.Contains(err.Error(),"event research:") {t.Fatalf("first Event failure lost: %v",err)}
 case <-time.After(2*time.Second):t.Fatal("parallel Market was not cancelled after Event failure")
 }
 select {
 case <-marketAgent.stopped:
 case <-time.After(time.Second):t.Fatal("Market did not observe cancellation")
 }
}

func TestInvalidEventContextCancelsParallelMarket(t *testing.T) {
 marketAgent:=&cancellationAwareMarket{started:make(chan struct{}),stopped:make(chan struct{})}
 eventAgent:=&fakeEventAgent{result:&eventagent.Context{},release:marketAgent.started}
 w,err:=New(context.Background(),Config{
  MarketAgent:marketAgent,TechnicalAgent:&fakeTechnicalAgent{result:validTechnicalContext()},
  EventAgent:eventAgent,Synthesizer:&fakeSynthesizer{},TradingAgent:&fakeArtifactTradingAgent{result:validDecision()},
 })
 if err!=nil {t.Fatal(err)}
 result:=make(chan error,1)
 go func(){_,err:=w.RunArtifact(context.Background(),validWorkflowInput());result<-err}()
 select {
 case err:=<-result:
  if err==nil || !strings.Contains(err.Error(),"invalid event context:") {t.Fatalf("event validation error lost: %v",err)}
 case <-time.After(2*time.Second):t.Fatal("Market not cancelled after invalid Event context")
 }
 select {
 case <-marketAgent.stopped:
 case <-time.After(time.Second):t.Fatal("Market did not observe cancellation")
 }
}
