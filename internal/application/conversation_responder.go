package application

import (
 "context"
 "errors"
 "strings"

 "github.com/cloudwego/eino/adk"
 "github.com/cloudwego/eino/schema"
)

// ConversationRunnerResponder is the transport-neutral bridge from a
// request-scoped authenticated endpoint to Eino's actual agent runtime.
type ConversationRunnerResponder struct { runner *adk.Runner }

func NewConversationRunnerResponder(ctx context.Context, agent adk.Agent) (*ConversationRunnerResponder,error) {
 if ctx==nil||agent==nil{return nil,errors.New("conversation agent and context are required")}
 return &ConversationRunnerResponder{runner:adk.NewRunner(ctx,adk.RunnerConfig{Agent:agent})},nil
}

func (r *ConversationRunnerResponder) Respond(ctx context.Context,message string)(string,error){
 if r==nil||r.runner==nil{return "",errors.New("conversation runtime unavailable")}
 if ctx==nil{return "",errors.New("conversation context is nil")}
 message=strings.TrimSpace(message)
 if message==""{return "",errors.New("conversation message required")}
 iter:=r.runner.Query(ctx,message)
 var answer string
 for {
  event,ok:=iter.Next()
  if !ok {break}
  if event.Err!=nil{return "",errors.New("conversation agent failed")}
  if event.Output==nil||event.Output.MessageOutput==nil||event.Output.MessageOutput.Message==nil{continue}
  msg:=event.Output.MessageOutput.Message
  if msg.Role==schema.Assistant&&len(msg.ToolCalls)==0&&strings.TrimSpace(msg.Content)!=""{answer=msg.Content}
 }
 if err:=ctx.Err();err!=nil{return "",err}
 if answer==""{return "",errors.New("conversation produced no answer")}
 return answer,nil
}
