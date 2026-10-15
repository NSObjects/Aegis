package httpapi

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "regexp"
 "strings"
)

type AuthorizedConversationResponder interface {
 RespondAuthorized(context.Context,string,string)(string,error)
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
  authorized,ok:=h.conversation.(AuthorizedConversationResponder)
  if !ok {http.Error(w,"research start unavailable",http.StatusServiceUnavailable);return}
  answer,err=authorized.RespondAuthorized(r.Context(),request.Message,request.StartSymbol)
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
