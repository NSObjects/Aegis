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
