package research

import (
 "bytes"
 "context"
 "errors"
 "strings"
 "testing"
)

func TestProgressStageSuccessAndFailure(t *testing.T) {
 var output bytes.Buffer
 ctx:=WithProgress(context.Background(),&output)
 finish:=startProgress(ctx,"Market")
 finish(nil)
 failing:=startProgress(ctx,"Event")
 err:=errors.New("secret provider message")
 failing(&err)
 log:=output.String()
 for _,want:=range []string{"stage=Market status=START","stage=Market status=DONE","stage=Event status=START","stage=Event status=FAIL","duration="}{
  if !strings.Contains(log,want){t.Fatalf("missing %q in %q",want,log)}
 }
 if strings.Contains(log,"secret provider message"){t.Fatal("provider errors must not appear in progress logs")}
}

func TestProgressIsOptIn(t *testing.T){
 finish:=startProgress(context.Background(),"Market")
 finish(nil)
}
