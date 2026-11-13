package application

import (
 "context"
 "errors"
 "sync/atomic"
 "testing"
 "time"

 "Aegis/internal/capability/technical"
)

// This verifies the application boundary, independent of provider HTTP
// fixtures. PoolManager may not start any scan when cache preflight fails.
type prefetchProbeResearcher struct{
 fail atomic.Bool
 ready atomic.Bool
 scanCalls atomic.Int32
 prefetchCalls atomic.Int32
}
func (p *prefetchProbeResearcher) PoolCacheEnabled()bool{return true}
func (p *prefetchProbeResearcher) PreparePool(ctx context.Context,asOf string,symbols []string,progress func(int,int)error)error {
 p.ready.Store(false)
 p.prefetchCalls.Add(1)
 if err:=progress(1,technical.MarketCacheSessionCount);err!=nil{return err}
 if p.fail.Load(){return errors.New("simulated incomplete market day snapshot")}
 if err:=progress(technical.MarketCacheSessionCount,technical.MarketCacheSessionCount);err!=nil{return err}
 p.ready.Store(true)
 return nil
}
func (p *prefetchProbeResearcher) Research(context.Context,technical.Request)(map[string]any,error){
 if !p.ready.Load(){return nil,errors.New("research called before cache was validated")}
 p.scanCalls.Add(1)
 return map[string]any{"status":"NO_DATA"},nil
}
func TestPoolCachePreflightStopsScannerAndResumesSafely(t *testing.T){
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 probe:=&prefetchProbeResearcher{}
 probe.fail.Store(true)
 manager,_:=newPoolTestManager(t,ctx,t.TempDir(),poolSymbols(3),probe)
 record,err:=manager.Start(ctx,"20261022")
 if err!=nil{t.Fatal(err)}
 stopped:=waitPool(t,manager,record.ID,PoolFailed)
 if stopped.FailureCode!="MARKET_CACHE_PREFETCH_FAILED"||stopped.NextOffset!=0||
  stopped.CacheDaysDone!=1||stopped.CacheDaysTotal!=technical.MarketCacheSessionCount||
  probe.scanCalls.Load()!=0 {t.Fatalf("cache failure produced scan or bad progress: %+v scanned=%d",stopped,probe.scanCalls.Load())}
 if _,err:=manager.Comparison(record.ID,0,50);!errors.Is(err,ErrPoolNotComplete){
  t.Fatalf("partial cache published candidates: %v",err)
 }
 probe.fail.Store(false)
 deadline:=time.Now().Add(3*time.Second)
 for {
  _,err=manager.Resume(record.ID)
  if err==nil{break}
  if !errors.Is(err,ErrPoolBusy)||time.Now().After(deadline){t.Fatal(err)}
  time.Sleep(20*time.Millisecond)
 }
 completed:=waitPool(t,manager,record.ID,PoolCompleted)
 if completed.NextOffset!=3||completed.CacheDaysDone!=technical.MarketCacheSessionCount||
  probe.scanCalls.Load()!=3||probe.prefetchCalls.Load()!=2 {
  t.Fatalf("cache resume did not finish full scan: %+v scanned=%d prepared=%d",completed,probe.scanCalls.Load(),probe.prefetchCalls.Load())
 }
}
