package application

import (
 "path/filepath"
 "testing"
 "time"

 "Aegis/internal/strategy/pullback"
)

func TestReconcileInterruptedScansOnStartup(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"scans")
 store,err:=NewFileScanStore(dir);if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Add(-time.Minute)
 states:=[]ScanState{ScanPending,ScanRunning,ScanFailed}
 ids:=[]string{"00000000000000000000000000000001","00000000000000000000000000000002","00000000000000000000000000000003"}
 for i,state:=range states{
  r:=ScanRecord{ID:ids[i],AsOf:"20261022",StrategyVersion:pullback.StrategyVersion,UniverseSource:"fixture",State:state,Symbols:[]string{"600519.SH"},CreatedAt:now,UpdatedAt:now}
  if state==ScanFailed{r.FailureCode="SCAN_EXECUTION_FAILED"}
  if err:=store.Save(r);err!=nil{t.Fatal(err)}
 }
 count,err:=store.ReconcileInterrupted();if err!=nil||count!=2{t.Fatalf("reconcile %d %v",count,err)}
 for i,id:=range ids{
  r,err:=store.Get(id);if err!=nil{t.Fatal(err)}
  if r.State!=ScanFailed{t.Fatalf("state %d = %s",i,r.State)}
  if i<2&&r.FailureCode!="PROCESS_INTERRUPTED"{t.Fatalf("missing interrupted reason: %+v",r)}
  if i==2&&r.FailureCode!="SCAN_EXECUTION_FAILED"{t.Fatal("terminal record mutated")}
 }
 count,err=store.ReconcileInterrupted();if err!=nil||count!=0{t.Fatalf("reconciliation not idempotent %d %v",count,err)}
}
