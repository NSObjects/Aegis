package application

import (
 "context"
 "errors"
 "path/filepath"
 "testing"
 "time"

 "Aegis/internal/capability/technical"
)

type scanStub struct { started chan struct{}; release chan struct{}; fail bool }
func (s *scanStub) Research(ctx context.Context,req technical.Request)(map[string]any,error){
 select {case s.started<-struct{}{}:default:}
 select {case <-s.release:case <-ctx.Done():return nil,ctx.Err()}
 if s.fail{return nil,errors.New("provider unavailable")}
 return map[string]any{"status":"NO_DATA"},nil
}
func TestScanManagerAsyncLifecycleAndFailure(t *testing.T){
 for _,fail:=range []bool{false,true}{
  store,err:=NewFileScanStore(filepath.Join(t.TempDir(),"private"));if err!=nil{t.Fatal(err)}
  src:=&scanStub{started:make(chan struct{},1),release:make(chan struct{}),fail:fail}
  manager,err:=NewScanManager(store,src);if err!=nil{t.Fatal(err)}
  r,err:=manager.Submit(context.Background(),"20261022","test",[]string{"600519.SH"})
  if err!=nil||r.State!=ScanPending{t.Fatalf("submit %+v %v",r,err)}
  <-src.started
  if _,err:=manager.Submit(context.Background(),"20261022","test",[]string{"000001.SZ"});err==nil{t.Fatal("concurrent scan accepted")}
  close(src.release)
  var final ScanRecord
  deadline:=time.After(3*time.Second)
  for {
   final,err=manager.Get(r.ID);if err!=nil{t.Fatal(err)}
   if final.State==ScanCompleted||final.State==ScanFailed{break}
   select {case <-deadline:t.Fatal("scan never reached terminal state");case <-time.After(10*time.Millisecond):}
  }
  if fail {
   if final.State!=ScanFailed||final.FailureCode!="SCAN_EXECUTION_FAILED"{t.Fatalf("provider error not recorded %+v",final)}
  }else if final.State!=ScanCompleted||len(final.Results)!=1||final.Results[0].Symbol!="600519.SH" {t.Fatalf("completed results invalid %+v",final)}
 }
}
