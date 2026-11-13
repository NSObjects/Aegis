package technical

import (
 "context"
 "errors"
)
func (r *HTTPResearcher) EnableMarketDayCache(dir string)error {
 if r==nil||r.api==nil{return errors.New("technical researcher must be initialized")}
 cache,err:=NewMarketDayCache(r.api,dir);if err!=nil{return err}
 r.poolMu.Lock();r.poolCache=cache;r.prepared=nil;r.poolMu.Unlock()
 return nil
}
func (r *HTTPResearcher) PoolCacheEnabled()bool {
 if r==nil{return false}
 r.poolMu.RLock();defer r.poolMu.RUnlock()
 return r.poolCache!=nil
}
func (r *HTTPResearcher) PreparePool(ctx context.Context,asOf string,symbols []string,progress func(int,int)error)error {
 if r==nil{return errors.New("nil technical researcher")}
 r.poolMu.Lock();cache:=r.poolCache;r.prepared=nil;r.poolMu.Unlock()
 if cache==nil{return nil}
 prepared,err:=cache.Prepare(ctx,asOf,symbols,progress)
 if err!=nil{return err}
 r.poolMu.Lock();r.prepared=prepared;r.poolMu.Unlock()
 return nil
}
