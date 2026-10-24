package httpapi

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "os"
 "strings"
 "time"

 "Aegis/internal/application"
)

// WatchlistStore is the persistence contract behind the observation pool.
// It stores operator-curated entries only; it never triggers research itself.
type WatchlistStore interface {
 Add(symbol, reason, conditions string)(application.WatchItem,error)
 Remove(symbol string)error
 List()([]application.WatchItem,error)
}
func (h *Handler) WithWatchlistStore(store WatchlistStore)*Handler{h.watchlist=store;return h}

// WatchlistRunRef links one entry to its most recent persisted research run.
// It is an opaque link for the UI, not a fresh research conclusion.
type WatchlistRunRef struct {
 RunID string `json:"run_id"`
 State application.RunState `json:"state"`
 Mode application.ResearchMode `json:"mode"`
 CreatedAt time.Time `json:"created_at"`
}
type watchlistItemView struct {
 application.WatchItem
 LatestRun *WatchlistRunRef `json:"latest_run"`
}

func (h *Handler) watchlistRoute(w http.ResponseWriter,r *http.Request){
 if h.watchlist==nil{http.Error(w,"watchlist unavailable",http.StatusServiceUnavailable);return}
 if r.URL.Path=="/api/v1/watchlist"{
  switch r.Method{
  case http.MethodGet:
   items,err:=h.watchlist.List()
   if err!=nil{http.Error(w,"watchlist unavailable",http.StatusInternalServerError);return}
   views,err:=h.attachLatestRuns(r.Context(),items)
   if err!=nil{http.Error(w,"research store unavailable",http.StatusInternalServerError);return}
   sendJSON(w,http.StatusOK,map[string]any{"items":views});return
  case http.MethodPost:
   h.watchlistAdd(w,r);return
  default:
   methodNotAllowed(w,"GET, POST");return
  }
 }
 symbol:=strings.TrimPrefix(r.URL.Path,"/api/v1/watchlist/")
 if symbol==""||strings.Contains(symbol,"/"){http.NotFound(w,r);return}
 if r.Method!=http.MethodDelete{methodNotAllowed(w,"DELETE");return}
 symbol=strings.ToUpper(symbol)
 if !application.ValidWatchSymbol(symbol){http.NotFound(w,r);return}
 if err:=h.watchlist.Remove(symbol);err!=nil{
  if errors.Is(err,os.ErrNotExist){http.NotFound(w,r);return}
  http.Error(w,"watchlist unavailable",http.StatusInternalServerError);return
 }
 w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) watchlistAdd(w http.ResponseWriter,r *http.Request){
 defer r.Body.Close()
 var req struct{
  Symbol string `json:"symbol"`
  Reason string `json:"reason"`
  Conditions string `json:"conditions"`
 }
 decoder:=json.NewDecoder(io.LimitReader(r.Body,(16<<10)+1))
 decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&req);err!=nil {http.Error(w,"invalid watchlist request",http.StatusBadRequest);return}
 var extra any
 if err:=decoder.Decode(&extra);!errors.Is(err,io.EOF){http.Error(w,"invalid watchlist request",http.StatusBadRequest);return}
 req.Symbol=strings.ToUpper(strings.TrimSpace(req.Symbol))
 item,err:=h.watchlist.Add(req.Symbol,strings.TrimSpace(req.Reason),strings.TrimSpace(req.Conditions))
 if err!=nil{
  switch{
  case errors.Is(err,application.ErrWatchlistDuplicate):
   http.Error(w,"watchlist symbol already present",http.StatusConflict)
  case errors.Is(err,application.ErrWatchlistFull):
   http.Error(w,"watchlist capacity reached",http.StatusConflict)
  default:
   http.Error(w,"invalid watchlist item",http.StatusBadRequest)
  }
  return
 }
 sendJSON(w,http.StatusCreated,watchlistItemView{WatchItem:item,LatestRun:nil})
}

// attachLatestRuns resolves each entry's most recent run through the existing
// RunController. A run-store failure fails the whole listing: serving entries
// with silently unknown research links would misstate their history.
func (h *Handler) attachLatestRuns(ctx context.Context,items []application.WatchItem)([]watchlistItemView,error){
 views:=make([]watchlistItemView,0,len(items))
 latest:=map[string]*WatchlistRunRef{}
 if len(items)>0{
  if h.runs==nil{return nil,errors.New("run controller required for watchlist links")}
  runs,err:=h.runs.List(ctx)
  if err!=nil{return nil,err}
  for i:=range runs{
   run:=runs[i]
   if !application.ValidWatchSymbol(run.Symbol){continue}
   ref:=&WatchlistRunRef{RunID:run.ID,State:run.State,Mode:run.Mode,CreatedAt:run.CreatedAt}
   current,ok:=latest[run.Symbol]
   if !ok||ref.CreatedAt.After(current.CreatedAt)||(ref.CreatedAt.Equal(current.CreatedAt)&&ref.RunID>current.RunID){latest[run.Symbol]=ref}
  }
 }
 for _,item:=range items{
  views=append(views,watchlistItemView{WatchItem:item,LatestRun:latest[item.Symbol]})
 }
 return views,nil
}
