package application

import (
 "path/filepath"
 "testing"
 "time"

 "Aegis/internal/strategy/pullback"
)
func TestFileScanStoreListNewestAndLimit(t *testing.T){
 store,err:=NewFileScanStore(filepath.Join(t.TempDir(),"scans"));if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 for i,id:=range []string{"00000000000000000000000000000001","00000000000000000000000000000002","00000000000000000000000000000003"}{
  at:=now.Add(time.Duration(i)*time.Minute)
  r:=ScanRecord{ID:id,AsOf:"20261022",StrategyVersion:pullback.StrategyVersion,State:ScanPending,UniverseSource:"USER_SUPPLIED",Symbols:[]string{"600519.SH"},CreatedAt:at,UpdatedAt:at}
  if err:=store.Save(r);err!=nil{t.Fatal(err)}
 }
 rows,err:=store.List(2);if err!=nil{t.Fatal(err)}
 if len(rows)!=2||rows[0].ID!="00000000000000000000000000000003"||rows[1].ID!="00000000000000000000000000000002"{t.Fatalf("unexpected history %+v",rows)}
 reopened,err:=NewFileScanStore(filepath.Join(store.dir));if err!=nil{t.Fatal(err)}
 all,err:=reopened.List(100);if err!=nil||len(all)!=3{t.Fatalf("reopened list count=%d err=%v",len(all),err)}
 if _,err=store.List(101);err==nil{t.Fatal("unbounded query accepted")}
}
