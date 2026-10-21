package httpapi

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "regexp"
 "strings"
 "time"
 "Aegis/internal/application"
)

// ConfirmedResearchSessionBinder is implemented only by the trusted Go runtime.
// This API currently supports a single server-authenticated operator.
type ConversationSessionResolver interface {
 ResolveConversationSession(context.Context,string,string)(application.ConversationSessionBinding,error)
}

type ConversationSessionHistoryResolver interface {
 ResolveConversationSessionHistory(context.Context,string,string)([]application.ConversationSessionBinding,error)
}

type ConfirmedResearchSessionBinder interface {
 BindCompletedConversationRun(context.Context,string,string,string)(application.ConversationSessionBinding,error)
}

type ConfirmedResearchStarter interface {
 StartConfirmedResearch(context.Context,string,string)(application.ConversationRunSummary,error)
}

var conversationMentionedSymbol = regexp.MustCompile(`(?:600|601|603|605|688)[0-9]{3}\.SH|(?:000|001|002|003|300|301|302)[0-9]{3}\.SZ`)
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
 SessionID string `json:"session_id,omitempty"`
 }
 if err:=dec.Decode(&request);err!=nil {http.Error(w,"invalid conversation request",http.StatusBadRequest);return}
 var extra any
 if dec.Decode(&extra)!=io.EOF {http.Error(w,"invalid or oversized conversation request",http.StatusBadRequest);return}
 request.Message=strings.TrimSpace(request.Message)
 if len(request.Message)==0||len(request.Message)>2048 {http.Error(w,"invalid conversation message",http.StatusBadRequest);return}
 var answer string
 var err error
 // A confirmed new Run may join an existing, authenticated session.
 // Resolve the owner before starting any potentially billable work.
 if request.SessionID!="" && (request.StartSymbol!="" || request.ConfirmStart) {
  if !request.ConfirmStart || !conversationStartSymbol.MatchString(request.StartSymbol) {http.Error(w,"invalid research authorization",http.StatusBadRequest);return}
  if application.ValidateConversationSessionID(request.SessionID)!=nil {http.Error(w,"invalid conversation session",http.StatusBadRequest);return}
  resolver,ok:=h.conversation.(ConversationSessionResolver)
  if !ok {http.Error(w,"conversation session unavailable",http.StatusServiceUnavailable);return}
  if _,resolveErr:=resolver.ResolveConversationSession(r.Context(),request.SessionID,"single-operator");resolveErr!=nil {
   http.Error(w,"conversation session not found",http.StatusNotFound);return
  }
  if _,ok:=h.conversation.(ConfirmedResearchSessionBinder);!ok {http.Error(w,"conversation session binder unavailable",http.StatusServiceUnavailable);return}
 }
 if request.StartSymbol!="" || request.ConfirmStart {
  if !request.ConfirmStart || !conversationStartSymbol.MatchString(request.StartSymbol) {http.Error(w,"invalid research authorization",http.StatusBadRequest);return}
  starter,ok:=h.conversation.(ConfirmedResearchStarter)
  if !ok {http.Error(w,"research start unavailable",http.StatusServiceUnavailable);return}
  run,startErr:=starter.StartConfirmedResearch(r.Context(),request.Message,request.StartSymbol)
  if startErr!=nil {http.Error(w,"research start failed",http.StatusServiceUnavailable);return}
  if run.RunID=="" || run.Symbol!=request.StartSymbol {http.Error(w,"invalid research start result",http.StatusBadGateway);return}
  response:=map[string]string{"answer":"研究任务已创建，请按 Run ID 查询状态。","run_id":run.RunID,"symbol":run.Symbol,"state":string(run.State)}
  response["session_binding"]="FAILED"
  if binder,ok:=h.conversation.(ConfirmedResearchSessionBinder);ok {
   sessionID:=request.SessionID
   var idErr error
   if sessionID=="" {sessionID,idErr=application.NewConversationSessionID()}
   if idErr==nil {
    if _,bindErr:=binder.BindCompletedConversationRun(r.Context(),sessionID,"single-operator",run.RunID);bindErr==nil {
     response["session_id"]=sessionID
     response["session_binding"]="BOUND"
    }
   }
  }
  if response["session_binding"]=="FAILED" {
   response["answer"]="研究任务已创建，但会话关联失败。请保存 Run ID 并通过研究列表查询；不要重复启动研究。"
  }
  sendJSON(w,http.StatusOK,response)
  return
 } else {
  if request.SessionID!="" {
   if application.ValidateConversationSessionID(request.SessionID)!=nil {http.Error(w,"invalid conversation session",http.StatusBadRequest);return}
   resolver,ok:=h.conversation.(ConversationSessionResolver)
   if !ok {http.Error(w,"conversation session unavailable",http.StatusServiceUnavailable);return}
   binding,resolveErr:=resolver.ResolveConversationSession(r.Context(),request.SessionID,"single-operator")
   if resolveErr!=nil {http.Error(w,"conversation session not found",http.StatusNotFound);return}
   if strings.Contains(request.Message,"上次") || strings.Contains(request.Message,"上一回") || refersToHistoricalResearchDay(request.Message) {
    historyResolver,ok:=h.conversation.(ConversationSessionHistoryResolver)
    if !ok {http.Error(w,"历史研究不可用，请指定 Run ID",http.StatusConflict);return}
    history,historyErr:=historyResolver.ResolveConversationSessionHistory(r.Context(),request.SessionID,"single-operator")
    if historyErr!=nil || len(history)==0 || history[0].RunID!=binding.RunID {
     http.Error(w,"历史研究引用不可用",http.StatusConflict);return
    }
    selected,selectErr:=selectConversationHistoricalRun(request.Message,history,time.Now())
    if selectErr!=nil {http.Error(w,"无法唯一定位历史研究，请指定 Run ID",http.StatusConflict);return}
    binding=selected
   }
   request.Message="当前已授权的历史研究引用：run_id="+binding.RunID+"，symbol="+binding.Symbol+"，as_of="+binding.AsOf.Format(time.RFC3339)+"。仅可使用研究只读工具验证事实；不得启动新研究。用户问题："+request.Message
  }
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

 
// Resolve one unambiguous reference from server-owned Run history.
// Relative dates use Asia/Shanghai calendar days, not exchange trading days.
func selectConversationHistoricalRun(message string,history []application.ConversationSessionBinding,now time.Time)(application.ConversationSessionBinding,error) {
 if len(history)==0{return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable}
 previous:=strings.Contains(message,"上次") || strings.Contains(message,"上一回")
 yesterday:=strings.Contains(message,"昨天") || strings.Contains(message,"昨日")
 beforeYesterday:=strings.Contains(message,"前天") || strings.Contains(message,"前日")
 periods:=0
 for _,present:=range []bool{previous,yesterday,beforeYesterday} {if present {periods++}}
 if periods!=1 || strings.Contains(message,"上周") || strings.Contains(message,"上个月") {
  return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable
 }
 // Exclude the current Run before filtering by ticker. Otherwise a current
 // Run for a different ticker incorrectly skips that ticker's latest history.
 if previous {history=history[1:]}
 symbols:=conversationMentionedSymbol.FindAllString(strings.ToUpper(message),-1)
 if len(symbols)>0 {
  ticker:=symbols[0]
  for _,symbol:=range symbols[1:] {if symbol!=ticker{return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable}}
  filtered:=make([]application.ConversationSessionBinding,0,len(history))
  for _,entry:=range history {if entry.Symbol==ticker {filtered=append(filtered,entry)}}
  history=filtered
  if len(history)==0{return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable}
 }
 if previous {
  if len(history)>0{return history[0],nil}
  return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable
 }
 days:=0
 switch {
 case strings.Contains(message,"前天"),strings.Contains(message,"前日"): days=2
 case strings.Contains(message,"昨天"),strings.Contains(message,"昨日"): days=1
 default: return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable
 }
 loc,err:=time.LoadLocation("Asia/Shanghai")
 if err!=nil{return application.ConversationSessionBinding{},err}
 target:=now.In(loc).AddDate(0,0,-days).Format("2006-01-02")
 var matched application.ConversationSessionBinding
 count:=0
 for _,entry:=range history {
  if entry.AsOf.IsZero(){continue}
  if entry.AsOf.In(loc).Format("2006-01-02")==target {
   matched=entry
   count++
  }
 }
 if count!=1{return application.ConversationSessionBinding{},application.ErrConversationSessionUnavailable}
 return matched,nil
}

func refersToHistoricalResearchDay(message string) bool {
 for _,term:=range []string{"昨天","前天","上周","上个月","昨日","前日"} {
  if strings.Contains(message,term) {return true}
 }
 return false
}
