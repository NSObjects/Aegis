package technical

import (
 "context"
 "errors"
 "testing"
)

type observedResearcher struct { err error }
func (r observedResearcher) Research(context.Context, Request) (map[string]any,error) {
 if r.err != nil { return nil,r.err }
 return map[string]any{"status":"ok"},nil
}

func TestTechnicalToolReportsRealInvocations(t *testing.T) {
 for _, tc := range []struct{name string; err error; want string}{
  {"success",nil,"DONE"}, {"failure",errors.New("secret provider detail"),"FAIL"},
 } {
  t.Run(tc.name,func(t *testing.T){
   invoked,err:=NewTool(observedResearcher{err:tc.err})
   if err!=nil {t.Fatal(err)}
   var got []ToolObservation
   ctx:=WithToolObserver(context.Background(),func(ev ToolObservation){got=append(got,ev)})
   _,err=invoked.InvokableRun(ctx,`{"symbol":"600519.SH","as_of":"20261012"}`)
   if (err!=nil)!=(tc.err!=nil) {t.Fatalf("run error %v, expected failure=%v",err,tc.err!=nil)}
   if len(got)!=2 || got[0].Status!="START" || got[1].Status!=tc.want { t.Fatalf("events: %+v",got) }
   if got[0].At.IsZero() || got[1].At.IsZero() || got[1].DurationMS<0 {t.Fatalf("invalid times: %+v",got)}
  })
 }
}
