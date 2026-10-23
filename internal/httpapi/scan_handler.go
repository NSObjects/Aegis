package httpapi

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "os"
 "strings"

 "Aegis/internal/application"
)

// ScanController is intentionally separate from the LLM ResearchRun controller.
type ScanController interface {
 Submit(context.Context,string,string,[]string)(application.ScanRecord,error)
 Get(string)(application.ScanRecord,error)
}
func (h *Handler) WithScanController(scans ScanController) *Handler {h.scans=scans;return h}

func (h *Handler) scanRoute(w http.ResponseWriter,r *http.Request){
 if h.scans==nil{http.Error(w,"scan service unavailable",http.StatusServiceUnavailable);return}
 if r.URL.Path=="/api/v1/scans"{
  if r.Method!=http.MethodPost{methodNotAllowed(w,"POST");return}
  defer r.Body.Close()
  var req struct{
   AsOf string `json:"as_of"`
   UniverseSource string `json:"universe_source"`
   Symbols []string `json:"symbols"`
  }
  decoder:=json.NewDecoder(io.LimitReader(r.Body,16<<10+1))
  decoder.DisallowUnknownFields()
  if err:=decoder.Decode(&req);err!=nil {http.Error(w,"invalid scan request",http.StatusBadRequest);return}
  var extra any
  if err:=decoder.Decode(&extra);!errors.Is(err,io.EOF){http.Error(w,"invalid scan request",http.StatusBadRequest);return}
  if req.UniverseSource==""||len(req.Symbols)==0||len(req.Symbols)>20 {
   http.Error(w,"invalid scan inputs",http.StatusBadRequest);return
  }
  result,err:=h.scans.Submit(r.Context(),req.AsOf,req.UniverseSource,req.Symbols)
  if err!=nil{
   if strings.Contains(err.Error(),"already running"){http.Error(w,"scan already running",http.StatusConflict);return}
   http.Error(w,"scan rejected",http.StatusBadRequest);return
  }
  sendJSON(w,http.StatusAccepted,result)
  return
 }
 id:=strings.TrimPrefix(r.URL.Path,"/api/v1/scans/")
 if r.Method!=http.MethodGet{methodNotAllowed(w,"GET");return}
 if !isRunID(id){http.NotFound(w,r);return}
 result,err:=h.scans.Get(id)
 if err!=nil{
  if errors.Is(err,os.ErrNotExist){http.NotFound(w,r);return}
  http.Error(w,"scan store unavailable",http.StatusInternalServerError);return
 }
 sendJSON(w,http.StatusOK,result)
}
