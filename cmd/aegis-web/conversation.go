package main

import (
 "context"
 "errors"
 "fmt"
 "strings"
 "time"

 "Aegis/internal/application"
 "Aegis/internal/config"
 "Aegis/internal/llm"
 "Aegis/internal/settings"
)

// conversationRuntime resolves the existing encrypted provider credentials
// for each request, so credential rotation doesn't require a server restart.
// It exposes only read tools: asking about old research cannot initiate a
// paid research run. Authorization is enforced by the outer HTTP handler.
type conversationRuntime struct {
 runs *application.RunManager
 credentials *settings.Store
 configPath string
 bindings *application.FileConversationBindingStore
}

func (r *conversationRuntime) Respond(ctx context.Context,message string)(string,error) {
 if r==nil||r.runs==nil||r.credentials==nil||strings.TrimSpace(r.configPath)=="" {
  return "",errors.New("conversation runtime unavailable")
 }
 if ctx==nil{return "",errors.New("conversation context required")}
 ctx,cancel:=context.WithTimeout(ctx,45*time.Second)
 defer cancel()
 secrets,err:=r.credentials.Load(ctx)
 if err!=nil{return "",fmt.Errorf("load conversation credentials: %w",err)}
 cfg,err:=config.Load(r.configPath)
 if err!=nil{return "",fmt.Errorf("load conversation model config: %w",err)}
 for name,profile:=range cfg.Models {
  if key:=strings.TrimSpace(secrets[profile.APIKeyEnv]);key!="" {
   profile.APIKey=key
   cfg.Models[name]=profile
  }
 }
 selected,err:=cfg.ModelProfileForAgent("conversation")
 if err!=nil{return "",err}
 chatModel,err:=llm.NewManager(cfg.Models).GetToolCalling(ctx,selected)
 if err!=nil{return "",fmt.Errorf("create conversation model: %w",err)}
 researchTools,err:=application.NewConversationResearchTools(r.runs)
 if err!=nil{return "",err}
 agent,err:=application.NewConversationAgent(ctx,chatModel,researchTools,nil)
 if err!=nil{return "",err}
 responder,err:=application.NewConversationRunnerResponder(ctx,agent)
 if err!=nil{return "",err}
 return responder.Respond(ctx,message)
}

// StartConfirmedResearch directly submits exactly one Run through the existing
// manager. Consent is validated by the authenticated HTTP handler; the model
// cannot skip, duplicate, or alter this durable side effect.
func (r *conversationRuntime) StartConfirmedResearch(ctx context.Context,_ string,symbol string)(application.ConversationRunSummary,error) {
 if r==nil||r.runs==nil {return application.ConversationRunSummary{},errors.New("research runtime unavailable")}
 if ctx==nil {return application.ConversationRunSummary{},errors.New("research context is nil")}
 cutoff:=time.Now().UTC()
 input,err:=application.NewLiveResearchInput(symbol,"",cutoff,7*24*time.Hour)
 if err!=nil {return application.ConversationRunSummary{},err}
 tools,err:=application.NewConversationResearchTools(r.runs)
 if err!=nil{return application.ConversationRunSummary{},err}
 return tools.StartResearch(ctx,application.ResearchRequest{Mode:application.ModeDecision,Input:input})
}

// BindCompletedConversationRun associates a freshly submitted, persisted Run
// with a server-validated conversation session. The caller must derive owner
// identity from authenticated server state, never from model output.
func (r *conversationRuntime) BindCompletedConversationRun(ctx context.Context, sessionID, ownerID, runID string) (application.ConversationSessionBinding,error) {
 if r==nil||r.bindings==nil {return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable}
 return r.bindings.Bind(ctx,sessionID,ownerID,runID)
}

func (r *conversationRuntime) ResolveConversationSession(ctx context.Context,sessionID,ownerID string)(application.ConversationSessionBinding,error) {
 if r==nil||r.bindings==nil {return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable}
 return r.bindings.Resolve(sessionID,ownerID)
}
