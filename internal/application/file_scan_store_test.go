package application

import (
 "testing"
 "time"

 "Aegis/internal/strategy/pullback"
)
func TestFileScanStoreLifecycleAndReopen(t *testing.T){
 dir:=t.TempDir();store,err:=NewFileScanStore(dir);if err!=nil{t.Fatal(err)}
 id,err:=NewScanID();if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 r:=ScanRecord{ID:id,AsOf:"20261022",StrategyVersion:pullback.StrategyVersion,State:ScanPending,UniverseSource:"TUSHARE_STOCK_BASIC_L_D_P",Symbols:[]string{"000001.SZ","600519.SH"},CreatedAt:now,UpdatedAt:now}
 if err:=store.Save(r);err!=nil{t.Fatal(err)}
 r.State=ScanRunning;r.UpdatedAt=now.Add(time.Second)
 if err:=store.Save(r);err!=nil{t.Fatal(err)}
 r.State=ScanCompleted;r.UpdatedAt=now.Add(2*time.Second)
 for _,symbol:=range r.Symbols {r.Results=append(r.Results,pullback.Result{Symbol:symbol,AsOf:r.AsOf,Version:r.StrategyVersion,Verdict:pullback.Rejected,Reason:"TREND_NOT_UP"})}
 if err:=store.Save(r);err!=nil{t.Fatal(err)}
 reopened,err:=NewFileScanStore(dir);if err!=nil{t.Fatal(err)}
 got,err:=reopened.Get(id)
 if err!=nil||len(got.Results)!=2||got.State!=ScanCompleted{t.Fatalf("reopen: %+v %v",got,err)}
 if err:=reopened.Save(r);err==nil{t.Fatal("terminal record overwritten")}
 r.State=ScanPending;r.Results=nil
 if err:=reopened.Save(r);err==nil{t.Fatal("terminal state rolled back")}
}
func TestFileScanStoreRejectsForgedResults(t *testing.T){
 now:=time.Now().UTC()
 base:=ScanRecord{ID:"0123456789abcdef0123456789abcdef",AsOf:"20261022",StrategyVersion:pullback.StrategyVersion,State:ScanCompleted,UniverseSource:"TUSHARE_STOCK_BASIC_L_D_P",Symbols:[]string{"600519.SH"},CreatedAt:now,UpdatedAt:now,
  Results:[]pullback.Result{{Symbol:"000001.SZ",AsOf:"20261022",Version:pullback.StrategyVersion,Verdict:pullback.Selected}}}
 if err:=validateScanRecord(base);err==nil{t.Fatal("cross-symbol result accepted")}
 base.Results[0].Symbol="600519.SH";base.Results[0].AsOf="20261023"
 if err:=validateScanRecord(base);err==nil{t.Fatal("wrong PIT date accepted")}
 base.Results=nil
 if err:=validateScanRecord(base);err==nil{t.Fatal("incomplete completed record accepted")}
}
