package application

import (
 "context"
 "errors"
 "sync"
 "time"
)

// ConversationSessionBinding records only a validated reference to an existing
// ResearchRun, never a second copy of its report, research evidence or credentials.
type ConversationSessionBinding struct {
 SessionID string
 OwnerID string
 RunID string
 Symbol string
 AsOf time.Time
}

type ConversationSessionBindings struct {
 mu sync.Mutex
 bySession map[string]ConversationSessionBinding
 runs interface { Get(context.Context,string)(RunRecord,error) }
}

var ErrConversationSessionUnavailable=errors.New("conversation session not available")

func NewConversationSessionBindings(runs interface{Get(context.Context,string)(RunRecord,error)})(*ConversationSessionBindings,error){
 if runs==nil{return nil,errors.New("research run lookup required")}
 return &ConversationSessionBindings{bySession:make(map[string]ConversationSessionBinding),runs:runs},nil
}

// Bind verifies that the supplied Run exists; the Run's own canonical symbol
// and cutoff override any client text. OwnerID must come from trusted auth,
// not a model argument. This in-memory core is not a durable session store.
func (s *ConversationSessionBindings) Bind(ctx context.Context,sessionID,ownerID,runID string)(ConversationSessionBinding,error){
 if s==nil||s.runs==nil{return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 if sessionID==""||ownerID==""||!validRunID(runID){return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 rec,err:=s.runs.Get(ctx,runID)
 if err!=nil{return ConversationSessionBinding{},err}
 binding:=ConversationSessionBinding{SessionID:sessionID,OwnerID:ownerID,RunID:rec.ID,Symbol:rec.Symbol,AsOf:rec.RequestedAsOf}
 s.mu.Lock();defer s.mu.Unlock()
 if existing,exists:=s.bySession[sessionID];exists&&existing.OwnerID!=ownerID{return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 s.bySession[sessionID]=binding
 return binding,nil
}

// Resolve refuses a foreign owner before exposing even the linked Run ID.
func (s *ConversationSessionBindings) Resolve(sessionID,ownerID string)(ConversationSessionBinding,error){
 if s==nil{return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 s.mu.Lock();defer s.mu.Unlock()
 binding,ok:=s.bySession[sessionID]
 if !ok||ownerID==""||binding.OwnerID!=ownerID{return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 return binding,nil
}
