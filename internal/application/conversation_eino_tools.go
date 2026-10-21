package application

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"

 "github.com/cloudwego/eino/components/tool"
 "github.com/cloudwego/eino/components/tool/utils"
)

// ConversationRunIDArgs is intentionally minimal: the model cannot supply
// a portfolio/account snapshot or rewrite an existing ResearchRun.
type ConversationRunIDArgs struct {
 RunID string `json:"run_id" jsonschema:"required,description=Existing persisted Aegis research Run ID"`
}
type ConversationListArgs struct { }
type ConversationCompareArgs struct {
 LeftRunID string `json:"left_run_id" jsonschema:"required,description=First completed Run ID"`
 RightRunID string `json:"right_run_id" jsonschema:"required,description=Second completed Run ID"`
}
type ConversationArtifactResult struct {
 RunID string `json:"run_id"`
 Artifact json.RawMessage `json:"artifact"`
}
type ConversationDebateResult struct {
 RunID string `json:"run_id"`
 Debate json.RawMessage `json:"debate_state"`
}

// EinoReadTools exposes real, execute-capable Eino tools for a Conversation
// Agent. Only read operations are exposed here; starting a paid run requires a
// separately validated and authorized request path.
func (t *ConversationResearchTools) EinoReadTools() ([]tool.InvokableTool, error) {
 if t == nil || t.runs == nil { return nil,errors.New("conversation tools unavailable") }
 get,err:=utils.InferTool("GetResearchRun","Get the status and failure code of an existing Aegis research run by run_id.",func(ctx context.Context,arg ConversationRunIDArgs)(ConversationRunSummary,error){
  return t.GetResearchRun(ctx,arg.RunID)
 })
 if err!=nil{return nil,fmt.Errorf("get run tool: %w",err)}
 list,err:=utils.InferTool("ListResearchRuns","List existing persisted Aegis research runs; do not imply that old research describes current prices.",func(ctx context.Context,_ ConversationListArgs)([]ConversationRunSummary,error){
  return t.ListResearchRuns(ctx)
 })
 if err!=nil{return nil,fmt.Errorf("list runs tool: %w",err)}
 artifact,err:=utils.InferTool("ReadArtifact","Read only the original completed research artifact by run_id; unavailable for pending, failed or cancelled runs.",func(ctx context.Context,arg ConversationRunIDArgs)(ConversationArtifactResult,error){
  data,err:=t.ReadArtifact(ctx,arg.RunID)
  if err!=nil{return ConversationArtifactResult{},err}
  return ConversationArtifactResult{RunID:arg.RunID,Artifact:data},nil
 })
 if err!=nil{return nil,fmt.Errorf("read artifact tool: %w",err)}
 debate,err:=utils.InferTool("ReadDebate","Read the saved Bull/Bear debate_state of a completed portfolio research run by run_id; never invent arguments.",func(ctx context.Context,arg ConversationRunIDArgs)(ConversationDebateResult,error){
  data,err:=t.ReadDebate(ctx,arg.RunID)
  if err!=nil{return ConversationDebateResult{},err}
  return ConversationDebateResult{RunID:arg.RunID,Debate:data},nil
 })
 if err!=nil{return nil,fmt.Errorf("read debate tool: %w",err)}
 compare,err:=utils.InferTool("CompareResearchRuns","Compare two completed DECISION research runs for the same symbol and mode. Returns persisted conclusion differences and PIT caveats; never infer causal explanations.",func(ctx context.Context,arg ConversationCompareArgs)(ConversationRunComparison,error){
  return t.CompareResearchRuns(ctx,arg.LeftRunID,arg.RightRunID)
 })
 if err!=nil{return nil,fmt.Errorf("compare runs tool: %w",err)}
 return []tool.InvokableTool{get,list,artifact,debate,compare},nil
}
