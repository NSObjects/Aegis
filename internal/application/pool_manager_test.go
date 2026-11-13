package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
	"Aegis/internal/strategy/pullback"
)

type poolFakeCalendar struct { open bool }
func (f poolFakeCalendar) IsTradingDay(context.Context,string)(bool,error){return f.open,nil}
type poolFakeUniverse struct { symbols []string; err error }
func (f poolFakeUniverse) ListStockUniverse(_ context.Context, asOf string) (technical.StockUniverse,error) {
	if f.err!=nil { return technical.StockUniverse{},f.err }
	return technical.StockUniverse{AsOf:asOf,Source:poolSource,Symbols:append([]string(nil),f.symbols...)},nil
}
type poolFakeResearcher struct { broken atomic.Bool; gate chan struct{}; started chan struct{} }
func (f *poolFakeResearcher) Research(ctx context.Context, _ technical.Request) (map[string]any,error) {
	if f.started!=nil { select {case f.started<-struct{}{}:default:} }
	if f.gate!=nil {
		select { case <-f.gate: case <-ctx.Done():return nil,ctx.Err() }
	}
	if f.broken.Load() { return nil,errors.New("simulated provider failure") }
	return map[string]any{"status":"NO_DATA"},nil
}
func poolSymbols(count int) []string {
	out:=make([]string,0,count)
	for i:=0;i<count;i++ { out=append(out,fmt.Sprintf("600%03d.SH",i)) }
	return out
}
func waitPool(t *testing.T, m *PoolManager, id string, want PoolState) PoolRecord {
	t.Helper()
	deadline:=time.Now().Add(10*time.Second)
	for time.Now().Before(deadline) {
		r,err:=m.Get(id)
		if err!=nil { t.Fatal(err) }
		if r.State==want { return r }
		if r.State==PoolFailed&&want!=PoolFailed { t.Fatalf("pool unexpectedly failed: %+v",r) }
		time.Sleep(20*time.Millisecond)
	}
	t.Fatalf("pool did not reach %s",want)
	return PoolRecord{}
}
func newPoolTestManager(t *testing.T, ctx context.Context, dir string, symbols []string, researcher technical.Researcher) (*PoolManager,*FilePoolStore) {
	t.Helper()
	scans,err:=NewFileScanStore(filepath.Join(dir,"scans"));if err!=nil{t.Fatal(err)}
	manager,err:=NewScanManager(scans,researcher);if err!=nil{t.Fatal(err)}
	store,err:=NewFilePoolStore(filepath.Join(dir,"pools"));if err!=nil{t.Fatal(err)}
	pool,err:=NewPoolManager(ctx,store,manager,poolFakeUniverse{symbols:symbols},poolFakeCalendar{open:true});if err!=nil{t.Fatal(err)}
	return pool,store
}
func TestPoolAllBatchesDurableAndCoverage(t *testing.T) {
	ctx,cancel:=context.WithCancel(context.Background());defer cancel()
	dir:=t.TempDir()
	pool,store:=newPoolTestManager(t,ctx,dir,poolSymbols(41),&poolFakeResearcher{})
	rec,err:=pool.Start(ctx,"20261022")
	if err!=nil { t.Fatal(err) }
	if rec.State!=PoolRunning||len(rec.Symbols)!=41||rec.UniverseSHA256=="" {t.Fatalf("bad frozen universe: %+v",rec)}
	if _,err:=pool.Start(ctx,"20261022");!errors.Is(err,ErrPoolBusy){t.Fatalf("concurrent universe allowed: %v",err)}
	if _,err:=pool.Comparison(rec.ID,0,50);!errors.Is(err,ErrPoolNotComplete){t.Fatalf("incomplete pool rank visible: %v",err)}
	done:=waitPool(t,pool,rec.ID,PoolCompleted)
	if done.NextOffset!=41||len(done.BatchScanIDs)!=3||done.ActiveScanID!="" {t.Fatalf("bad coverage: %+v",done)}
	v,err:=pool.Comparison(rec.ID,0,50)
	if err!=nil {t.Fatal(err)}
	if v.UniverseCount!=41||v.SelectedCount!=0||v.IncomparableCount!=41||
		v.ReasonCounts["NO_VALID_HISTORICAL_OBSERVATION"]!=41||len(v.Ranked)!=0 {t.Fatalf("incomparable coverage: %+v",v)}
	other,err:=NewFilePoolStore(filepath.Join(dir,"pools"))
	if err!=nil {t.Fatal(err)}
	persisted,err:=other.Get(rec.ID)
	if err!=nil||persisted.State!=PoolCompleted||persisted.UniverseSHA256!=done.UniverseSHA256 {
		t.Fatalf("pool failed durable roundtrip: %+v %v",persisted,err)
	}
	if _,err:=pool.Resume(rec.ID);!errors.Is(err,ErrPoolState){t.Fatalf("completed pool resumed: %v",err)}
	bad:=persisted;bad.Symbols=append([]string(nil),bad.Symbols...);bad.Symbols[0]="000001.SZ"
	if err:=store.Save(bad);err==nil {t.Fatal("changed frozen PIT universe accepted")}
}

func TestPoolExplicitPauseAndResume(t *testing.T) {
	ctx,cancel:=context.WithCancel(context.Background());defer cancel()
	researcher:=&poolFakeResearcher{gate:make(chan struct{}),started:make(chan struct{},1)}
	pool,_:=newPoolTestManager(t,ctx,t.TempDir(),poolSymbols(21),researcher)
	rec,err:=pool.Start(ctx,"20261022");if err!=nil{t.Fatal(err)}
	select {case <-researcher.started:case <-time.After(3*time.Second):t.Fatal("first batch not started")}
	if _,err:=pool.Pause(rec.ID);err!=nil{t.Fatal(err)}
	close(researcher.gate)
	pause:=waitPool(t,pool,rec.ID,PoolPaused)
	if pause.NextOffset!=20||len(pause.BatchScanIDs)!=1 {t.Fatalf("pause crossed batch boundary: %+v",pause)}
	// The worker exits before a fresh start is accepted; retry transient busy.
	var resumed PoolRecord
	deadline:=time.Now().Add(2*time.Second)
	for {
		resumed,err=pool.Resume(rec.ID)
		if err==nil {break}
		if !errors.Is(err,ErrPoolBusy)||time.Now().After(deadline){t.Fatal(err)}
		time.Sleep(20*time.Millisecond)
	}
	if resumed.State!=PoolRunning {t.Fatalf("not running: %+v",resumed)}
	done:=waitPool(t,pool,rec.ID,PoolCompleted)
	if done.NextOffset!=21||len(done.BatchScanIDs)!=2 {t.Fatalf("resume lost batch: %+v",done)}
}
func TestPoolFailedBatchRequiresExplicitResume(t *testing.T) {
	ctx,cancel:=context.WithCancel(context.Background());defer cancel()
	src:=&poolFakeResearcher{}
	src.broken.Store(true)
	pool,_:=newPoolTestManager(t,ctx,t.TempDir(),poolSymbols(1),src)
	rec,err:=pool.Start(ctx,"20261022");if err!=nil{t.Fatal(err)}
	failed:=waitPool(t,pool,rec.ID,PoolFailed)
	if failed.NextOffset!=0||failed.FailureCode!="BATCH_FAILED" {t.Fatalf("failed pool advanced: %+v",failed)}
	src.broken.Store(false)
	deadline:=time.Now().Add(2*time.Second)
	for {
		_,err=pool.Resume(rec.ID)
		if err==nil {break}
		if !errors.Is(err,ErrPoolBusy)||time.Now().After(deadline){t.Fatal(err)}
		time.Sleep(20*time.Millisecond)
	}
	done:=waitPool(t,pool,rec.ID,PoolCompleted)
	if len(done.BatchScanIDs)!=1 {t.Fatalf("bad resumed batches %+v",done)}
}
func TestPoolStoreInterruptedAndTamperedMembership(t *testing.T) {
	dir:=t.TempDir()
	store,err:=NewFilePoolStore(filepath.Join(dir,"pools"));if err!=nil{t.Fatal(err)}
	now:=time.Now().UTC()
	rec:=PoolRecord{ID:strings.Repeat("a",32),AsOf:"20261022",StrategyVersion:pullback.StrategyVersion,
		UniverseSource:poolSource,Symbols:poolSymbols(2),State:PoolRunning,
		CreatedAt:now,UpdatedAt:now,BatchScanIDs:[]string{}}
	rec.UniverseSHA256=poolDigest(rec.AsOf,rec.UniverseSource,rec.Symbols)
	if err:=store.Save(rec);err!=nil{t.Fatal(err)}
	ctx,cancel:=context.WithCancel(context.Background());defer cancel()
	scans,err:=NewFileScanStore(filepath.Join(dir,"scans"));if err!=nil{t.Fatal(err)}
	scanManager,err:=NewScanManager(scans,&poolFakeResearcher{});if err!=nil{t.Fatal(err)}
	_,err=NewPoolManager(ctx,store,scanManager,poolFakeUniverse{symbols:rec.Symbols},poolFakeCalendar{open:true})
	if err!=nil{t.Fatal(err)}
	got,err:=store.Get(rec.ID);if err!=nil{t.Fatal(err)}
	if got.State!=PoolPaused||got.FailureCode!="PROCESS_INTERRUPTED"{t.Fatalf("did not reconcile: %+v",got)}
	if _,err:=store.Get("../bad");err==nil {t.Fatal("invalid path accepted")}
}

func TestPoolRejectsNonTradingDayBeforeFetchingUniverse(t *testing.T) {
	ctx:=context.Background()
	dir:=t.TempDir()
	scanStore,err:=NewFileScanStore(filepath.Join(dir,"scans"));if err!=nil{t.Fatal(err)}
	scans,err:=NewScanManager(scanStore,&poolFakeResearcher{});if err!=nil{t.Fatal(err)}
	store,err:=NewFilePoolStore(filepath.Join(dir,"pools"));if err!=nil{t.Fatal(err)}
	pool,err:=NewPoolManager(ctx,store,scans,poolFakeUniverse{err:errors.New("should never fetch")},poolFakeCalendar{open:false})
	if err!=nil{t.Fatal(err)}
	if _,err:=pool.Start(ctx,"20261114");!errors.Is(err,ErrPoolNonTradingDay){t.Fatalf("nontrading day accepted: %v",err)}
	history,err:=pool.List(10)
	if err!=nil||len(history)!=0{t.Fatalf("invalid jobs created: %+v %v",history,err)}
}
