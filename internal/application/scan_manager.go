package application

import (
 "context"
 "errors"
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
 results,err:=pullback.ScanResearcher(ctx,m.researcher,r.AsOf,r.Symbols)
 r.UpdatedAt=time.Now().UTC()
 if err!=nil {
  r.State=ScanFailed;r.FailureCode="SCAN_EXECUTION_FAILED"
 }else {
  r.State=ScanCompleted;r.Results=results
 }
 _=m.store.Save(r)
}

func (m *ScanManager) Get(id string)(ScanRecord,error){
 if m==nil{return ScanRecord{},errors.New("nil scan manager")}
 return m.store.Get(id)
}
