package application

import (
 "context"
 "errors"
 "strings"
 "testing"
 "time"
)

func TestConversationSessionBindingScopesCanonicalRun(t *testing.T) {
 ctx:=context.Background()
 asOf:=time.Date(2026,10,16,0,0,0,0,time.UTC)
 id:=strings.Repeat("a",32)
 lookup:=&conversationRunsStub{records:map[string]RunRecord{id:{ID:id,Symbol:"600519.SH",RequestedAsOf:asOf,State:RunCompleted}}}
 bindings,err:=NewConversationSessionBindings(lookup)
 if err!=nil{t.Fatal(err)}
 saved,err:=bindings.Bind(ctx,"session-a","owner-one",id)
 if err!=nil{t.Fatal(err)}
 if saved.RunID!=id||saved.Symbol!="600519.SH"||!saved.AsOf.Equal(asOf){t.Fatalf("not canonical: %+v",saved)}
 got,err:=bindings.Resolve("session-a","owner-one")
 if err!=nil||got!=saved{t.Fatalf("resolve owner: %+v %v",got,err)}
 if _,err:=bindings.Resolve("session-a","owner-two");!errors.Is(err,ErrConversationSessionUnavailable){t.Fatalf("foreign owner read: %v",err)}
 if _,err:=bindings.Bind(ctx,"session-a","owner-two",id);!errors.Is(err,ErrConversationSessionUnavailable){t.Fatalf("foreign owner override: %v",err)}
 if got,err:=bindings.Resolve("session-a","owner-one");err!=nil||got!=saved{t.Fatalf("foreign owner mutated mapping: %+v %v",got,err)}
 if _,err:=bindings.Bind(ctx,"session-b","owner-one",strings.Repeat("b",32));!errors.Is(err,ErrRunNotFound){t.Fatalf("missing run accepted: %v",err)}
 for _,args:=range [][3]string{{"","owner-one",id},{"session-c","",id},{"session-c","owner-one","invalid"}} {
  if _,err:=bindings.Bind(ctx,args[0],args[1],args[2]);!errors.Is(err,ErrConversationSessionUnavailable){t.Fatalf("invalid association accepted: %v",err)}
 }
 if _,err:=bindings.Resolve("session-c","owner-one");!errors.Is(err,ErrConversationSessionUnavailable){t.Fatalf("missing session accepted: %v",err)}
}
