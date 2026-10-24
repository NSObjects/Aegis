package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"Aegis/internal/application"
	"Aegis/internal/capability/technical"
)

// ScanController enables isolated httptest coverage without a live data source.
type ScanController interface {
	Submit(context.Context, application.ScanRequest) (application.ScanRecord, error)
	Get(context.Context, string) (application.ScanRecord, error)
	List(context.Context) ([]application.ScanRecord, error)
	Cancel(context.Context, string) error
	GetSnapshot(ctx context.Context, scanID, symbol string) ([]technical.HistoricalBar, error)
}

// WithScanController wires the deterministic screening scan service. Without
// it the scan routes report the service as unavailable.
func (h *Handler) WithScanController(scans ScanController) *Handler {
	h.scans = scans
	return h
}

// scanRoutes dispatches /api/v1/scans and its subresources.
func (h *Handler) scanRoutes(w http.ResponseWriter, r *http.Request) {
	if h.scans == nil {
		http.Error(w, "scan service unavailable", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/scans")
	if path == "" || path == "/" {
		switch r.Method {
		case http.MethodPost:
			h.scanSubmit(w, r)
		case http.MethodGet:
			h.scanList(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	id := parts[0]
	if !isRunID(id) {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		h.scanGet(w, r, id)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		h.scanCancel(w, r, id)
		return
	}
	if len(parts) == 3 && parts[1] == "results" && r.Method == http.MethodGet {
		h.scanResult(w, r, id, parts[2])
		return
	}
	if len(parts) == 4 && parts[1] == "results" && parts[3] == "snapshot" && r.Method == http.MethodGet {
		h.scanSnapshot(w, r, id, parts[2])
		return
	}
	http.NotFound(w, r)
}

// shanghaiToday yields the A-share market's current calendar date. The zone
// has no daylight saving transitions, so the fixed offset is stable.
func shanghaiToday(now time.Time) string {
	return now.In(time.FixedZone("AsiaShanghai", 8*3600)).Format("20060102")
}

func (h *Handler) scanSubmit(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) > 1 || (len(keys) == 1 && keys[0] == "") {
		http.Error(w, "invalid Idempotency-Key", http.StatusBadRequest)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if err := application.ValidateIdempotencyKey(key); err != nil {
		http.Error(w, "invalid Idempotency-Key", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxCreateBytes+1))
	if err != nil || len(body) > maxCreateBytes {
		http.Error(w, "invalid or oversized request", http.StatusRequestEntityTooLarge)
		return
	}
	var payload struct {
		AsOf    string   `json:"as_of"`
		Symbols []string `json:"symbols,omitempty"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		http.Error(w, "invalid scan request", http.StatusBadRequest)
		return
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		http.Error(w, "trailing JSON not allowed", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.AsOf) == "" {
		payload.AsOf = shanghaiToday(h.now())
	}
	if err := application.ValidateScanAsOf(payload.AsOf); err != nil {
		http.Error(w, "invalid scan as_of", http.StatusBadRequest)
		return
	}
	for _, symbol := range payload.Symbols {
		if err := application.ValidateScanSymbol(strings.ToUpper(strings.TrimSpace(symbol))); err != nil {
			http.Error(w, "invalid scan symbol", http.StatusBadRequest)
			return
		}
	}
	rec, err := h.scans.Submit(r.Context(), application.ScanRequest{IdempotencyKey: key, AsOf: payload.AsOf, Symbols: payload.Symbols})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidScanRequest):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, application.ErrScanIdempotencyConflict):
			http.Error(w, "Idempotency-Key is already bound to different scan parameters", http.StatusConflict)
		case errors.Is(err, application.ErrScanCapacity):
			http.Error(w, "scan capacity reached", http.StatusTooManyRequests)
		case errors.Is(err, application.ErrScanManagerClosed):
			http.Error(w, "scan service unavailable", http.StatusServiceUnavailable)
		case errors.Is(err, context.Canceled):
			http.Error(w, "request canceled", http.StatusRequestTimeout)
		default:
			http.Error(w, "unable to submit scan", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Location", "/api/v1/scans/"+rec.ID)
	sendJSON(w, http.StatusAccepted, publicScan(rec))
}

func scanLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, application.ErrScanNotFound) {
		http.Error(w, "strategy scan not found", http.StatusNotFound)
	} else if errors.Is(err, context.Canceled) {
		http.Error(w, "request canceled", http.StatusRequestTimeout)
	} else {
		http.Error(w, "scan store unavailable", http.StatusInternalServerError)
	}
}

// publicScan removes internal request-correlation metadata from one record.
func publicScan(rec application.ScanRecord) application.ScanRecord {
	rec.IdempotencyDigest = ""
	rec.RequestFingerprint = ""
	return rec
}

// publicScanList additionally drops per-symbol payloads from listing rows.
func publicScanList(rec application.ScanRecord) application.ScanRecord {
	rec = publicScan(rec)
	rec.Results = nil
	rec.Events = nil
	return rec
}

func (h *Handler) scanList(w http.ResponseWriter, r *http.Request) {
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
	state := query.Get("state")
	switch state {
	case "", "PENDING", "RUNNING", "COMPLETED", "FAILED", "CANCELLED":
	default:
		http.Error(w, "invalid state filter", http.StatusBadRequest)
		return
	}
	v, err := h.scans.List(r.Context())
	if err != nil {
		scanLookupError(w, err)
		return
	}
	filtered := make([]application.ScanRecord, 0)
	for _, rec := range v {
		if state != "" && string(rec.State) != state {
			continue
		}
		filtered = append(filtered, rec)
	}
	total := len(filtered)
	if offset >= total {
		filtered = filtered[:0]
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		filtered = filtered[offset:end]
	}
	out := make([]application.ScanRecord, 0, len(filtered))
	for _, rec := range filtered {
		out = append(out, publicScanList(rec))
	}
	sendJSON(w, http.StatusOK, map[string]any{"scans": out, "total": total, "limit": limit, "offset": offset, "has_more": offset+len(out) < total})
}

func (h *Handler) scanLoad(w http.ResponseWriter, r *http.Request, id string) (application.ScanRecord, bool) {
	rec, err := h.scans.Get(r.Context(), id)
	if err != nil {
		scanLookupError(w, err)
		return application.ScanRecord{}, false
	}
	return rec, true
}

func (h *Handler) scanGet(w http.ResponseWriter, r *http.Request, id string) {
	if rec, ok := h.scanLoad(w, r, id); ok {
		sendJSON(w, http.StatusOK, publicScan(rec))
	}
}

func (h *Handler) scanCancel(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.scans.Cancel(r.Context(), id); err != nil {
		if errors.Is(err, application.ErrScanNotActive) {
			http.Error(w, "scan is not active", http.StatusConflict)
		} else {
			scanLookupError(w, err)
		}
		return
	}
	sendJSON(w, http.StatusAccepted, map[string]string{"status": "CANCELLATION_REQUESTED"})
}

func (h *Handler) scanResult(w http.ResponseWriter, r *http.Request, id, symbol string) {
	rec, ok := h.scanLoad(w, r, id)
	if !ok {
		return
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	for _, item := range rec.Results {
		if item.Symbol == symbol {
			sendJSON(w, http.StatusOK, map[string]any{"result": item})
			return
		}
	}
	http.Error(w, "scan result for symbol not found", http.StatusNotFound)
}

func (h *Handler) scanSnapshot(w http.ResponseWriter, r *http.Request, id, symbol string) {
	bars, err := h.scans.GetSnapshot(r.Context(), id, symbol)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrScanNotFound):
			http.Error(w, "strategy scan not found", http.StatusNotFound)
		case errors.Is(err, application.ErrScanSymbolNotFound):
			http.Error(w, "scan result for symbol not found", http.StatusNotFound)
		case errors.Is(err, application.ErrScanNotTerminal):
			http.Error(w, "scan snapshots are unavailable until the scan reaches a terminal state", http.StatusConflict)
		case errors.Is(err, context.Canceled):
			http.Error(w, "request canceled", http.StatusRequestTimeout)
		default:
			http.Error(w, "scan snapshot unavailable", http.StatusInternalServerError)
		}
		return
	}
	sendJSON(w, http.StatusOK, map[string]any{
		"scan_id":     id,
		"symbol":      strings.ToUpper(strings.TrimSpace(symbol)),
		"price_basis": "RAW",
		"bars":        bars,
	})
}
