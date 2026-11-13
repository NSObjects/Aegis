package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"Aegis/internal/application"
)

// PoolController is independent of research/LLM runs. All writes are explicit
// operator actions and all data remains protected by Handler's Bearer auth.
type PoolController interface {
	Start(context.Context,string) (application.PoolRecord,error)
	Resume(string) (application.PoolRecord,error)
	Pause(string) (application.PoolRecord,error)
	Get(string) (application.PoolRecord,error)
	List(int) ([]application.PoolRecord,error)
	Comparison(string,int,int) (application.PoolComparison,error)
}
func (h *Handler) WithPoolController(pools PoolController) *Handler { h.pools=pools;return h }

func (h *Handler) poolRoute(w http.ResponseWriter,r *http.Request) {
	if h.pools==nil { http.Error(w,"pool service unavailable",http.StatusServiceUnavailable);return }
	if r.URL.Path=="/api/v1/pools" {
		if r.Method==http.MethodGet {
			limit:=20
			if raw:=r.URL.Query().Get("limit");raw!="" {
				n,err:=strconv.Atoi(raw)
				if err!=nil||n<1||n>100 { http.Error(w,"invalid limit",http.StatusBadRequest);return }
				limit=n
			}
			if len(r.URL.Query())>1 { http.Error(w,"invalid filter",http.StatusBadRequest);return }
			pools,err:=h.pools.List(limit)
			if err!=nil { http.Error(w,"pool store unavailable",http.StatusInternalServerError);return }
			// History omits full universe membership; GET by ID returns the
			// immutable frozen membership if the operator needs an audit.
			type summary struct {
				ID string `json:"pool_id"`
				AsOf string `json:"as_of"`
				State application.PoolState `json:"state"`
				StrategyVersion string `json:"strategy_version"`
				UniverseSource string `json:"universe_source"`
				UniverseSHA256 string `json:"universe_sha256"`
				Count int `json:"universe_count"`
				NextOffset int `json:"next_offset"`
				CacheDaysDone int `json:"cache_days_done"`
				CacheDaysTotal int `json:"cache_days_total"`
				FailureCode string `json:"failure_code,omitempty"`
				CreatedAt interface{} `json:"created_at"`
			}
			out:=make([]summary,0,len(pools))
			for _,p:=range pools { out=append(out,summary{ID:p.ID,AsOf:p.AsOf,State:p.State,
				StrategyVersion:p.StrategyVersion,UniverseSource:p.UniverseSource,UniverseSHA256:p.UniverseSHA256,
				Count:len(p.Symbols),NextOffset:p.NextOffset,CacheDaysDone:p.CacheDaysDone,CacheDaysTotal:p.CacheDaysTotal,
    FailureCode:p.FailureCode,CreatedAt:p.CreatedAt}) }
			sendJSON(w,http.StatusOK,map[string]any{"pools":out})
			return
		}
		if r.Method!=http.MethodPost { methodNotAllowed(w,"GET, POST");return }
		defer r.Body.Close()
		var input struct{ AsOf string `json:"as_of"` }
		d:=json.NewDecoder(io.LimitReader(r.Body,1025))
		d.DisallowUnknownFields()
		if d.Decode(&input)!=nil { http.Error(w,"invalid pool request",http.StatusBadRequest);return }
		var extra any
		if !errors.Is(d.Decode(&extra),io.EOF) { http.Error(w,"invalid pool request",http.StatusBadRequest);return }
		record,err:=h.pools.Start(r.Context(),input.AsOf)
		if err!=nil {
			if errors.Is(err,application.ErrPoolBusy) { http.Error(w,"pool already running",http.StatusConflict);return }
			if errors.Is(err,application.ErrPoolNonTradingDay) { http.Error(w,"cutoff is not a trading day",http.StatusUnprocessableEntity);return }
			http.Error(w,"pool start failed",http.StatusBadRequest);return
		}
		sendJSON(w,http.StatusAccepted,record)
		return
	}
	parts:=strings.Split(strings.TrimPrefix(r.URL.Path,"/api/v1/pools/"),"/")
	if len(parts)<1||len(parts)>2||!isRunID(parts[0]) { http.NotFound(w,r);return }
	id:=parts[0]
	if len(parts)==2 && (parts[1]=="resume"||parts[1]=="pause") {
		if r.Method!=http.MethodPost { methodNotAllowed(w,"POST");return }
		var record application.PoolRecord
		var err error
		if parts[1]=="resume" { record,err=h.pools.Resume(id) } else { record,err=h.pools.Pause(id) }
		if err!=nil {
			if errors.Is(err,application.ErrPoolBusy)||errors.Is(err,application.ErrPoolState) { http.Error(w,"pool state conflict",http.StatusConflict);return }
			if errors.Is(err,os.ErrNotExist) { http.NotFound(w,r);return }
			http.Error(w,"pool unavailable",http.StatusInternalServerError);return
		}
		sendJSON(w,http.StatusAccepted,record)
		return
	}
	if r.Method!=http.MethodGet { methodNotAllowed(w,"GET");return }
	if len(parts)==2 && parts[1]=="comparison" {
		offset,limit:=0,50
		if raw:=r.URL.Query().Get("offset");raw!="" {
			n,err:=strconv.Atoi(raw);if err!=nil||n<0||n>4500 { http.Error(w,"invalid offset",http.StatusBadRequest);return };offset=n
		}
		if raw:=r.URL.Query().Get("limit");raw!="" {
			n,err:=strconv.Atoi(raw);if err!=nil||n<1||n>100 { http.Error(w,"invalid limit",http.StatusBadRequest);return };limit=n
		}
		if len(r.URL.Query())>2 { http.Error(w,"invalid filter",http.StatusBadRequest);return }
		view,err:=h.pools.Comparison(id,offset,limit)
		if err!=nil {
			if errors.Is(err,application.ErrPoolNotComplete) { http.Error(w,"pool incomplete",http.StatusConflict);return }
			if errors.Is(err,os.ErrNotExist) { http.NotFound(w,r);return }
			http.Error(w,"pool comparison unavailable",http.StatusInternalServerError);return
		}
		sendJSON(w,http.StatusOK,view);return
	}
	if len(parts)!=1 { http.NotFound(w,r);return }
	record,err:=h.pools.Get(id)
	if err!=nil {
		if errors.Is(err,os.ErrNotExist) { http.NotFound(w,r);return }
		http.Error(w,"pool unavailable",http.StatusInternalServerError);return
	}
	sendJSON(w,http.StatusOK,record)
}
