package application

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestFileConversationBindingStoreRestartAndIsolation(t *testing.T){
 ctx:=context.Background()
 id:=strings.Repeat("a",32)
 asOf:=time.Date(2026,10,16,1,0,0,0,time.UTC)
 runs:=&conversationRunsStub{records:map[string]RunRecord{id:{ID:id,Symbol:"600519.SH",RequestedAsOf:asOf}}}
 dir:=filepath.Join(t.TempDir(),"session")
 store,err:=NewFileConversationBindingStore(dir,runs);if err!=nil{t.Fatal(err)}
 saved,err:=store.Bind(ctx,"session-12345","operator-one",id);if err!=nil{t.Fatal(err)}
 if saved.RunID!=id||saved.Symbol!="600519.SH"||!saved.AsOf.Equal(asOf){t.Fatalf("wrong canonical ref: %+v",saved)}
 reopened,err:=NewFileConversationBindingStore(dir,runs);if err!=nil{t.Fatal(err)}
 got,err:=reopened.Resolve("session-12345","operator-one");if err!=nil||got!=saved{t.Fatalf("lost binding after restart: %+v %v",got,err)}
 if _,err:=reopened.Resolve("session-12345","other-owner");!errors.Is(err,ErrConversationSessionUnavailable){t.Fatalf("foreign read: %v",err)}
 if _,err:=reopened.Bind(ctx,"session-12345","other-owner",id);!errors.Is(err,ErrConversationSessionUnavailable){t.Fatalf("foreign overwrite: %v",err)}
 if _,err:=reopened.Bind(ctx,"session-54321","operator-one",strings.Repeat("b",32));!errors.Is(err,ErrRunNotFound){t.Fatalf("unpersisted run accepted: %v",err)}
 if _,err:=reopened.Resolve("session-54321","operator-one");!errors.Is(err,ErrConversationSessionUnavailable){t.Fatalf("phantom session: %v",err)}
 files,err:=os.ReadDir(dir);if err!=nil||len(files)!=1{t.Fatalf("unexpected persisted files %v: %v",files,err)}
 info,err:=os.Stat(filepath.Join(dir,files[0].Name()));if err!=nil||info.Mode().Perm()!=0600{t.Fatalf("session file not private: %v %v",info,err)}
}
func TestFileConversationBindingStoreRejectsPublicDirectory(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"sessions")
 if err:=os.Mkdir(dir,0755);err!=nil{t.Fatal(err)}
 runs:=&conversationRunsStub{}
 if _,err:=NewFileConversationBindingStore(dir,runs);err==nil{t.Fatal("public directory accepted")}
}

func TestFileConversationBindingStoreHistoryPersistsAndScopesOwner(t *testing.T) {
 ctx:=context.Background()
 first,second:=strings.Repeat("a",32),strings.Repeat("b",32)
 runs:=&conversationRunsStub{records:map[string]RunRecord{
  first:{ID:first,Symbol:"600519.SH",RequestedAsOf:time.Date(2026,10,17,0,0,0,0,time.UTC)},
  second:{ID:second,Symbol:"000001.SZ",RequestedAsOf:time.Date(2026,10,19,0,0,0,0,time.UTC)},
 }}
 dir:=filepath.Join(t.TempDir(),"sessions")
 store,err:=NewFileConversationBindingStore(dir,runs);if err!=nil{t.Fatal(err)}
 for _,id:=range []string{first,second,second} {
  if _,err:=store.Bind(ctx,"session-history","owner-one",id);err!=nil{t.Fatal(err)}
 }
 reopened,err:=NewFileConversationBindingStore(dir,runs);if err!=nil{t.Fatal(err)}
 history,err:=reopened.ResolveHistory("session-history","owner-one")
 if err!=nil||len(history)!=2||history[0].RunID!=second||history[1].RunID!=first {
  t.Fatalf("unexpected Run history: %+v %v",history,err)
 }
 if history[0].Symbol!="000001.SZ"||history[1].Symbol!="600519.SH"{t.Fatalf("lost canonical symbol: %+v",history)}
 if _,err:=reopened.ResolveHistory("session-history","other-owner");!errors.Is(err,ErrConversationSessionUnavailable){t.Fatalf("foreign history access: %v",err)}
 if _,err:=reopened.ResolveHistory("missing-session","owner-one");!errors.Is(err,ErrConversationSessionUnavailable){t.Fatalf("unknown history: %v",err)}
 current,err:=reopened.Resolve("session-history","owner-one")
 if err!=nil||current.RunID!=second{t.Fatalf("existing single Run lookup broke: %+v %v",current,err)}
}
