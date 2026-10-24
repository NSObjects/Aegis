// Package httpapi exposes ResearchRun as a minimal authenticated single-user
// HTTP API. It never constructs a second graph and never authorizes orders.
package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
 "sync"
	"time"

	"Aegis/internal/application"
	"Aegis/internal/portfolio"
	"Aegis/internal/settings"
)

const maxCreateBytes = 16 << 10
const apiRequestsPerMinute = 120
const maxConcurrentSSE = 16

var symbolPattern = regexp.MustCompile(`^[0-9]{6}\.(SH|SZ)$`)

// RunController enables isolated httptest coverage without paid providers.
type RunController interface {
	Submit(context.Context, application.ResearchRequest) (application.RunRecord, error)
	Get(context.Context, string) (application.RunRecord, error)
	List(context.Context) ([]application.RunRecord, error)
	Cancel(context.Context, string) error
}

// Handler keeps the API token on the server. Browser clients should proxy
// through a server-side Next.js adapter, never expose it in client JS.
type Handler struct {
	runs     RunController
	scans    ScanController
	token    string
	mode     application.ResearchMode
	lookback time.Duration
	now      func() time.Time
	poll     time.Duration
	localDev bool
	credentials *settings.Store
 conversation ConversationResponder
 rateMu sync.Mutex
 windowStart time.Time
 windowCount int
 sseSlots chan struct{}
}

func NewHandler(runs RunController, token string, mode application.ResearchMode, lookback time.Duration) (*Handler, error) {
	if runs == nil {
		return nil, errors.New("run controller is required")
	}
	if len(token) < 24 {
		return nil, errors.New("server API token must be at least 24 bytes")
	}
	if mode != application.ModeDecision && mode != application.ModePortfolio {
		return nil, errors.New("invalid API research mode")
	}
	if lookback <= 0 {
		return nil, errors.New("event lookback must be positive")
	}
	return &Handler{runs: runs, token: token, mode: mode, lookback: lookback, now: func() time.Time { return time.Now().UTC() }, poll: 300 * time.Millisecond, sseSlots: make(chan struct{}, maxConcurrentSSE)}, nil
}

// NewLocalDevHandler is permitted only by a loopback-bound CLI listener.
// A production HTTP server must always use NewHandler with a real bearer token.
func NewLocalDevHandler(runs RunController, mode application.ResearchMode, lookback time.Duration) (*Handler, error) {
	h, err := NewHandler(runs, strings.Repeat("aegis-local-dev-", 2), mode, lookback)
	if err != nil {
		return nil, err
	}
	h.localDev = true
	h.token = ""
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Equal length comparison avoids leaking token prefixes.
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !h.localDev && (!strings.HasPrefix(auth, prefix) || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, prefix)), []byte(h.token)) != 1) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !h.localDev && !h.allowRequest(time.Now()) {
  http.Error(w, "request rate limit exceeded", http.StatusTooManyRequests)
  return
 }
 if r.URL.Path == "/api/v1/conversation/query" { h.conversationQuery(w, r); return }
 if strings.HasPrefix(r.URL.Path,"/api/v1/conversation/sessions/") { h.conversationSessionRead(w,r); return }
 if r.URL.Path == "/api/v1/settings/credentials" { h.credentialSettings(w, r); return }
 if strings.HasPrefix(r.URL.Path, "/api/v1/scans") { h.scanRoutes(w, r); return }
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/research/runs")
	if path == r.URL.Path {
		http.NotFound(w, r)
		return
	}
	if path == "" || path == "/" {
		switch r.Method {
		case http.MethodPost:
			h.submit(w, r)
		case http.MethodGet:
			h.list(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}
		return
	}
	rest := strings.TrimPrefix(path, "/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	if !isRunID(id) {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		h.get(w, r, id)
		return
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "artifact":
			if r.Method == http.MethodGet {
				h.artifact(w, r, id)
				return
			}
		case "events":
			if r.Method == http.MethodGet {
				h.events(w, r, id)
				return
			}
		case "stream":
			if r.Method == http.MethodGet {
				h.stream(w, r, id)
				return
			}
		case "cancel":
			if r.Method == http.MethodPost {
				h.cancel(w, r, id)
				return
			}
		}
	}
	http.NotFound(w, r)
}

// A bounded per-process, single-user admission window; behind a reverse proxy
// distributed deployments require a shared limiter instead.
func (h *Handler) allowRequest(now time.Time) bool {
 h.rateMu.Lock()
 defer h.rateMu.Unlock()
 if h.windowStart.IsZero() || now.Sub(h.windowStart) >= time.Minute {
  h.windowStart = now
  h.windowCount = 0
 }
 if h.windowCount >= apiRequestsPerMinute { return false }
 h.windowCount++
 return true
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
func isRunID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}

func sendJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func publicRun(rec application.RunRecord) application.RunRecord {
	rec.IdempotencyDigest = ""
	rec.RequestFingerprint = ""
	rec.Artifact = nil
	rec.Events = nil
	return rec
}
func lookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, application.ErrRunNotFound) {
		http.Error(w, "research run not found", http.StatusNotFound)
	} else if errors.Is(err, context.Canceled) {
		http.Error(w, "request canceled", http.StatusRequestTimeout)
	} else {
		http.Error(w, "research store unavailable", http.StatusInternalServerError)
	}
}
func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	keys := r.Header.Values("Idempotency-Key")
	if len(keys)>1 || (len(keys)==1 && keys[0]=="") {
		http.Error(w,"invalid Idempotency-Key",http.StatusBadRequest)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if err:=application.ValidateIdempotencyKey(key);err!=nil {
		http.Error(w,"invalid Idempotency-Key",http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxCreateBytes+1))
	if err != nil || len(body) > maxCreateBytes {
		http.Error(w, "invalid or oversized request", http.StatusRequestEntityTooLarge)
		return
	}
	var payload struct {
		Symbol      string                   `json:"symbol"`
		Mode        application.ResearchMode `json:"mode"`
		MarketQuery string                   `json:"market_query,omitempty"`
		RerunOf     string                   `json:"rerun_of,omitempty"`
		Portfolio   *portfolio.Context       `json:"portfolio,omitempty"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		http.Error(w, "invalid research request", http.StatusBadRequest)
		return
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		http.Error(w, "trailing JSON not allowed", http.StatusBadRequest)
		return
	}
	if payload.Mode == "" {
		payload.Mode = h.mode
	}
	if payload.Mode != h.mode {
		http.Error(w, "research mode unavailable on this server", http.StatusBadRequest)
		return
	}
	payload.Symbol = strings.ToUpper(strings.TrimSpace(payload.Symbol))
	if !symbolPattern.MatchString(payload.Symbol) {
		http.Error(w, "invalid A-share symbol", http.StatusBadRequest)
		return
	}
	if len(payload.MarketQuery) > 1024 {
		http.Error(w, "market query is too long", http.StatusBadRequest)
		return
	}
	if h.mode == application.ModeDecision && payload.Portfolio != nil {
		http.Error(w, "decision mode cannot accept account data", http.StatusBadRequest)
		return
	}
	if h.mode == application.ModePortfolio {
		if payload.Portfolio == nil {
			http.Error(w, "explicit portfolio state is required; use NOT_PROVIDED if unknown", http.StatusBadRequest)
			return
		}
		if err := portfolio.Validate(payload.Portfolio); err != nil {
			http.Error(w, "invalid portfolio snapshot", http.StatusBadRequest)
			return
		}
	}
	input, err := application.NewLiveResearchInput(payload.Symbol, payload.MarketQuery, h.now(), h.lookback)
	if err != nil {
		http.Error(w, "invalid research input", http.StatusBadRequest)
		return
	}
	rec, err := h.runs.Submit(r.Context(), application.ResearchRequest{IdempotencyKey:key, Mode: payload.Mode, Input: input, Portfolio: payload.Portfolio, RerunOf: payload.RerunOf})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrRunIdempotencyConflict):
			http.Error(w, "Idempotency-Key is already bound to different research parameters", http.StatusConflict)
		case errors.Is(err, application.ErrInvalidRerunSource):
			http.Error(w, "invalid or unavailable rerun source", http.StatusBadRequest)
		case errors.Is(err, application.ErrRunCapacity):
			http.Error(w, "research capacity reached", http.StatusTooManyRequests)
		case errors.Is(err, application.ErrRunManagerClosed):
			http.Error(w, "research service unavailable", http.StatusServiceUnavailable)
		default:
			http.Error(w, "unable to submit research", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Location", "/api/v1/research/runs/"+rec.ID)
	sendJSON(w, http.StatusAccepted, publicRun(rec))
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, offset := 100, 0
	if raw := query.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			http.Error(w, "limit must be between 1 and 100", http.StatusBadRequest)
			return
		}
		limit = n
	}
	if raw := query.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 1000000 {
			http.Error(w, "invalid offset", http.StatusBadRequest)
			return
		}
		offset = n
	}
	symbol := strings.ToUpper(strings.TrimSpace(query.Get("symbol")))
	if symbol != "" && (len(symbol) > 9 || !regexp.MustCompile(`^[0-9A-Z.]+$`).MatchString(symbol)) {
		http.Error(w, "invalid symbol filter", http.StatusBadRequest)
		return
	}
	state := query.Get("state")
	switch state {
	case "", "PENDING", "RUNNING", "COMPLETED", "FAILED", "CANCELLED":
	default:
		http.Error(w, "invalid state filter", http.StatusBadRequest)
		return
	}
	v, err := h.runs.List(r.Context())
	if err != nil {
		lookupError(w, err)
		return
	}
	filtered := make([]application.RunRecord, 0)
	for _, rec := range v {
		if symbol != "" && !strings.Contains(rec.Symbol, symbol) { continue }
		if state != "" && string(rec.State) != state { continue }
		filtered = append(filtered, rec)
	}
	total := len(filtered)
	if offset >= total { filtered = filtered[:0] } else {
		end := offset + limit
		if end > total { end = total }
		filtered = filtered[offset:end]
	}
	out := make([]application.RunRecord, 0, len(filtered))
	for _, rec := range filtered { out = append(out, publicRun(rec)) }
	sendJSON(w, http.StatusOK, map[string]any{"runs": out, "total": total, "limit": limit, "offset": offset, "has_more": offset+len(out) < total})
}
func (h *Handler) load(w http.ResponseWriter, r *http.Request, id string) (application.RunRecord, bool) {
	rec, err := h.runs.Get(r.Context(), id)
	if err != nil {
		lookupError(w, err)
		return application.RunRecord{}, false
	}
	return rec, true
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request, id string) {
	rec, ok := h.load(w, r, id)
	if ok {
		sendJSON(w, http.StatusOK, publicRun(rec))
	}
}
func (h *Handler) artifact(w http.ResponseWriter, r *http.Request, id string) {
	rec, ok := h.load(w, r, id)
	if !ok {
		return
	}
	if rec.State != application.RunCompleted || len(rec.Artifact) == 0 {
		http.Error(w, "artifact unavailable until research completes", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(rec.Artifact)
}
func (h *Handler) events(w http.ResponseWriter, r *http.Request, id string) {
	rec, ok := h.load(w, r, id)
	if ok {
		sendJSON(w, http.StatusOK, map[string]any{"events": rec.Events})
	}
}
func (h *Handler) cancel(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.runs.Cancel(r.Context(), id); err != nil {
		if errors.Is(err, application.ErrRunNotActive) {
			http.Error(w, "research is not active", http.StatusConflict)
		} else {
			lookupError(w, err)
		}
		return
	}
	sendJSON(w, http.StatusAccepted, map[string]string{"status": "CANCELLATION_REQUESTED"})
}
func terminal(s application.RunState) bool {
	return s == application.RunCompleted || s == application.RunFailed || s == application.RunCancelled
}
func (h *Handler) stream(w http.ResponseWriter, r *http.Request, id string) {
 select {
 case h.sseSlots <- struct{}{}:
  defer func(){ <-h.sseSlots }()
 default:
  http.Error(w, "SSE connection capacity reached", http.StatusTooManyRequests)
  return
 }
	last := uint64(0)
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		if len(raw) > 20 {
			http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
			return
		}
		n, e := strconv.ParseUint(raw, 10, 64)
		if e != nil {
			http.Error(w, "invalid Last-Event-ID", http.StatusBadRequest)
			return
		}
		last = n
	}
	rec, ok := h.load(w, r, id)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	ticker := time.NewTicker(h.poll)
	defer ticker.Stop()
	for {
		for _, ev := range rec.Events {
			if ev.Seq <= last {
				continue
			}
			b, e := json.Marshal(ev)
			if e != nil {
				return
			}
			if _, e = fmt.Fprintf(w, "id: %d\nevent: research_stage\ndata: %s\n\n", ev.Seq, b); e != nil {
				return
			}
			last = ev.Seq
		}
		flusher.Flush()
		if terminal(rec.State) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			v, e := h.runs.Get(r.Context(), id)
			if e != nil {
				return
			}
			rec = v
		}
	}
}
