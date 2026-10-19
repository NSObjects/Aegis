package httpapi

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "regexp"
 "strings"
 "Aegis/internal/application"
)

// ConfirmedResearchSessionBinder is implemented only by the trusted Go runtime.
// This API currently supports a single server-authenticated operator.
type ConversationSessionResolver interface {
 ResolveConversationSession(context.Context,string,string)(application.ConversationSessionBinding,error)
}

type ConfirmedResearchSessionBinder interface {
 BindCompletedConversationRun(context.Context,string,string,string)(application.ConversationSessionBinding,error)
}

type ConfirmedResearchStarter interface {
 StartConfirmedResearch(context.Context,string,string)(application.ConversationRunSummary,error)
}

var conversationStartSymbol = regexp.MustCompile(`^(?:(?:600|601|603|605|688)[0-9]{3}\.SH|(?:000|001|002|003|300|301|302)[0-9]{3}\.SZ)$`)

// ConversationResponder runs the conversation using approved model and tools.
type ConversationResponder interface {
 Respond(context.Context,string)(string,error)
}

// WithConversationResponder enables conversation only if a trusted server
// caller has explicitly configured its runtime.
func (h *Handler) WithConversationResponder(responder ConversationResponder) {
 if h!=nil {h.conversation=responder}
}
func (h *Handler) conversationQuery(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPost {methodNotAllowed(w,"POST");return}
 if h.conversation==nil {http.Error(w,"conversation unavailable",http.StatusServiceUnavailable);return}
 defer r.Body.Close()
 const maxConversationBytes=4096
 dec:=json.NewDecoder(io.LimitReader(r.Body,maxConversationBytes+1))
 dec.DisallowUnknownFields()
 var request struct {
 Message string `json:"message"`
 StartSymbol string `json:"start_symbol,omitempty"`
 ConfirmStart bool `json:"confirm_start,omitempty"`
 }
 if err:=dec.Decode(&request);err!=nil {http.Error(w,"invalid conversation request",http.StatusBadRequest);return}
 var extra any
 if dec.Decode(&extra)!=io.EOF {http.Error(w,"invalid or oversized conversation request",http.StatusBadRequest);return}
 request.Message=strings.TrimSpace(request.Message)
 if len(request.Message)==0||len(request.Message)>2048 {http.Error(w,"invalid conversation message",http.StatusBadRequest);return}
 var answer string
 var err error
 if request.StartSymbol!="" || request.ConfirmStart {
  if !request.ConfirmStart || !conversationStartSymbol.MatchString(request.StartSymbol) {http.Error(w,"invalid research authorization",http.StatusBadRequest);return}
  starter,ok:=h.conversation.(ConfirmedResearchStarter)
  if !ok {http.Error(w,"research start unavailable",http.StatusServiceUnavailable);return}
  run,startErr:=starter.StartConfirmedResearch(r.Context(),request.Message,request.StartSymbol)
  if startErr!=nil {http.Error(w,"research start failed",http.StatusServiceUnavailable);return}
  if run.RunID=="" || run.Symbol!=request.StartSymbol {http.Error(w,"invalid research start result",http.StatusBadGateway);return}
  response:=map[string]string{"answer":"研究任务已创建，请按 Run ID 查询状态。","run_id":run.RunID,"symbol":run.Symbol,"state":string(run.State)}
  if binder,ok:=h.conversation.(ConfirmedResearchSessionBinder);ok {
   sessionID,idErr:=application.NewConversationSessionID()
   if idErr==nil {
    if _,bindErr:=binder.BindCompletedConversationRun(r.Context(),sessionID,"single-operator",run.RunID);bindErr==nil {response["session_id"]=sessionID}
   }
  }
  sendJSON(w,http.StatusOK,response)
  return
 } else {
  answer,err=h.conversation.Respond(r.Context(),request.Message)
 }
 if err!=nil {
  if errors.Is(err,context.Canceled)||errors.Is(err,context.DeadlineExceeded) {http.Error(w,"conversation interrupted",http.StatusGatewayTimeout);return}
  http.Error(w,"conversation unavailable",http.StatusServiceUnavailable);return
 }
 if strings.TrimSpace(answer)=="" {http.Error(w,"conversation returned no answer",http.StatusServiceUnavailable);return}
 sendJSON(w,http.StatusOK,map[string]string{"answer":answer})
}

// conversationSessionRead exposes a canonical stored reference, not an artifact.
// The owner is fixed to this explicitly single-operator server boundary.
func (h *Handler) conversationSessionRead(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodGet {methodNotAllowed(w,"GET");return}
 resolver,ok:=h.conversation.(ConversationSessionResolver)
 if !ok {http.Error(w,"conversation sessions unavailable",http.StatusServiceUnavailable);return}
 id:=strings.TrimPrefix(r.URL.Path,"/api/v1/conversation/sessions/")
 if application.ValidateConversationSessionID(id)!=nil {http.Error(w,"invalid session",http.StatusBadRequest);return}
 binding,err:=resolver.ResolveConversationSession(r.Context(),id,"single-operator")
 if err!=nil {http.Error(w,"session not found",http.StatusNotFound);return}
 sendJSON(w,http.StatusOK,map[string]any{"session_id":binding.SessionID,"run_id":binding.RunID,"symbol":binding.Symbol,"as_of":binding.AsOf})
}
