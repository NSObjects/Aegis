package research

import (
 "context"
 "fmt"
 "io"
 "sync"
 "time"
)

type progressKey struct{}
type progressWriter struct{ mu sync.Mutex; out io.Writer }

// WithProgress enables safe stage-level progress reporting for a single workflow
// invocation. Only stage names, duration and status are emitted; no prompts,
// account balances, credentials or research content are logged.
func WithProgress(ctx context.Context, out io.Writer) context.Context {
 if out==nil {return ctx}
 return context.WithValue(ctx,progressKey{},&progressWriter{out:out})
}

func startProgress(ctx context.Context,stage string) func(*error) {
 p,_:=ctx.Value(progressKey{}).(*progressWriter)
 if p==nil {return func(*error){}}
 started:=time.Now()
 p.mu.Lock()
 fmt.Fprintf(p.out,"[Aegis] stage=%s status=START\n",stage)
 p.mu.Unlock()
 return func(err *error){
  status:="DONE"
  if err!=nil && *err!=nil {status="FAIL"}
  p.mu.Lock()
  fmt.Fprintf(p.out,"[Aegis] stage=%s status=%s duration=%s\n",stage,status,time.Since(started).Round(time.Millisecond))
  p.mu.Unlock()
 }
}
