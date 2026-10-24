package application

import (
 "context"
 "errors"
 "os"
 "sort"
 "sync"
 "time"

 "Aegis/internal/capability/technical"
 "Aegis/internal/strategy/pullback"
)

// ScanManager persists each transition and executes one bounded scan at a time.
// This is a single-process runner; interrupted RUNNING scans are not resumed.
type ScanManager struct {
 store *FileScanStore
 researcher technical.Researcher
 mu sync.Mutex
 busy bool
}

func NewScanManager(store *FileScanStore,researcher technical.Researcher)(*ScanManager,error){
 if store==nil||researcher==nil{return nil,errors.New("scan store and researcher required")}
 if _,err:=store.ReconcileInterrupted();err!=nil{return nil,err}
 return &ScanManager{store:store,researcher:researcher},nil
}

func (m *ScanManager) Submit(ctx context.Context,asOf,source string,symbols []string)(ScanRecord,error){
 if m==nil{return ScanRecord{},errors.New("nil scan manager")}
 m.mu.Lock()
 if m.busy {m.mu.Unlock();return ScanRecord{},errors.New("scan already running")}
 m.busy=true
 sorted:=append([]string(nil),symbols...)
 sort.Strings(sorted)
 id,err:=NewScanID()
 if err!=nil{m.busy=false;m.mu.Unlock();return ScanRecord{},err}
 now:=time.Now().UTC()
 record:=ScanRecord{ID:id,AsOf:asOf,StrategyVersion:pullback.StrategyVersion,State:ScanPending,UniverseSource:source,Symbols:sorted,CreatedAt:now,UpdatedAt:now}
 if err:=m.store.Save(record);err!=nil{m.busy=false;m.mu.Unlock();return ScanRecord{},err}
 m.mu.Unlock()
 // The asynchronous task survives HTTP request completion but has a bounded lifetime.
 go func(){runCtx,cancel:=context.WithTimeout(context.WithoutCancel(ctx),3*time.Minute);defer cancel();m.execute(runCtx,record)}()
 return record,nil
}

func (m *ScanManager) execute(ctx context.Context,r ScanRecord){
 defer func(){m.mu.Lock();m.busy=false;m.mu.Unlock()}()
 r.State=ScanRunning;r.UpdatedAt=time.Now().UTC()
 if err:=m.store.Save(r);err!=nil{return}
 // Persist each symbol's exact validated observation as it feeds evaluation;
 // symbols without a valid observation keep no snapshot and stay queryable
 // through their INSUFFICIENT_DATA verdict.
 snapshots:=make([]ScanSnapshotRef,0,len(r.Symbols))
 var persistErr error
 results,err:=pullback.ScanResearcher(ctx,m.researcher,r.AsOf,r.Symbols,func(o technical.HistoricalObservation){
  if persistErr!=nil{return}
  digest,e:=m.store.SaveSnapshot(r.ID,o)
  if e!=nil{persistErr=e;return}
  snapshots=append(snapshots,ScanSnapshotRef{Symbol:o.Symbol,SHA256:digest,BarsCount:len(o.Bars),EffectiveAsOf:o.EffectiveAsOf})
 })
 r.UpdatedAt=time.Now().UTC()
 switch {
 case persistErr!=nil:
  // A snapshot that cannot be durably anchored must fail the scan: exposing
  // verdicts whose inputs cannot be re-verified would break replayability.
  r.State=ScanFailed;r.FailureCode="SCAN_PERSISTENCE_FAILED"
 case err!=nil:
  r.State=ScanFailed;r.FailureCode="SCAN_EXECUTION_FAILED"
 default:
  r.State=ScanCompleted;r.Results=results;r.Snapshots=snapshots
 }
 _=m.store.Save(r)
}

// ErrScanNotCompleted and ErrScanSnapshotNotFound separate terminal-state and
// missing-symbol snapshot lookups for HTTP status mapping.
var (
 ErrScanNotCompleted=errors.New("scan snapshots require a terminal scan")
 ErrScanSnapshotNotFound=errors.New("scan snapshot not found for symbol")
)

// GetSnapshot returns the persisted input observation behind one verdict.
func (m *ScanManager) GetSnapshot(scanID,symbol string)(technical.HistoricalObservation,error){
 if m==nil{return technical.HistoricalObservation{},errors.New("nil scan manager")}
 record,err:=m.store.Get(scanID)
 if err!=nil{return technical.HistoricalObservation{},err}
 if record.State!=ScanCompleted{return technical.HistoricalObservation{},ErrScanNotCompleted}
 for _,ref:=range record.Snapshots{
  if ref.Symbol!=symbol{continue}
  o,err:=m.store.GetSnapshot(scanID,symbol,ref.SHA256)
  if os.IsNotExist(err){return technical.HistoricalObservation{},ErrScanSnapshotNotFound}
  return o,err
 }
 return technical.HistoricalObservation{},ErrScanSnapshotNotFound
}

func (m *ScanManager) Get(id string)(ScanRecord,error){
 if m==nil{return ScanRecord{},errors.New("nil scan manager")}
 return m.store.Get(id)
}

func (m *ScanManager) List(limit int)([]ScanRecord,error){
 if m==nil{return nil,errors.New("nil scan manager")}
 return m.store.List(limit)
}
