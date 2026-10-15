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

// RespondAuthorized exposes StartResearch only within this single explicit,
// confirmed-symbol request. The user must opt in through the HTTP DTO.
func (r *conversationRuntime) RespondAuthorized(ctx context.Context,message,symbol string)(string,error) {
 if r==nil||r.runs==nil||r.credentials==nil||strings.TrimSpace(r.configPath)=="" {return "",errors.New("conversation runtime unavailable")}
 if ctx==nil{return "",errors.New("conversation context required")}
 ctx,cancel:=context.WithTimeout(ctx,45*time.Second)
 defer cancel()
 secrets,err:=r.credentials.Load(ctx)
 if err!=nil{return "",err}
 cfg,err:=config.Load(r.configPath)
 if err!=nil{return "",err}
 for name,profile:=range cfg.Models {if key:=strings.TrimSpace(secrets[profile.APIKeyEnv]);key!="" {profile.APIKey=key;cfg.Models[name]=profile}}
 selected,err:=cfg.ModelProfileForAgent("conversation")
 if err!=nil{return "",err}
 chatModel,err:=llm.NewManager(cfg.Models).GetToolCalling(ctx,selected)
 if err!=nil{return "",err}
 researchTools,err:=application.NewConversationResearchTools(r.runs)
 if err!=nil{return "",err}
 agent,err:=application.NewConversationAgent(ctx,chatModel,researchTools,&application.ConversationStartPolicy{Cutoff:time.Now().UTC(),Lookback:7*24*time.Hour,AllowedSymbol:symbol})
 if err!=nil{return "",err}
 responder,err:=application.NewConversationRunnerResponder(ctx,agent)
 if err!=nil{return "",err}
 return responder.Respond(ctx,message)
}
