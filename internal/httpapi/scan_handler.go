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
	"Aegis/internal/capability/technical"
)

// ScanController is intentionally separate from the LLM ResearchRun controller.
type ScanController interface {
	Submit(context.Context, string, string, []string) (application.ScanRecord, error)
	Get(string) (application.ScanRecord, error)
	List(int) ([]application.ScanRecord, error)
	GetSnapshot(scanID, symbol string) (technical.HistoricalObservation, error)
	Comparison(scanID string) (application.ScanComparison, error)
	Replay(scanID string) (application.ScanReplay, error)
}

func (h *Handler) WithScanController(scans ScanController) *Handler { h.scans = scans; return h }

func (h *Handler) scanRoute(w http.ResponseWriter, r *http.Request) {
	if h.scans == nil {
		http.Error(w, "scan service unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.URL.Path == "/api/v1/scans" {
		if r.Method == http.MethodGet {
			limit := 20
			if raw := r.URL.Query().Get("limit"); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil || n < 1 || n > 100 {
					http.Error(w, "invalid limit", http.StatusBadRequest)
					return
				}
				limit = n
			}
			records, err := h.scans.List(limit)
			if err != nil {
				http.Error(w, "scan store unavailable", http.StatusInternalServerError)
				return
			}
			sendJSON(w, http.StatusOK, map[string]any{"scans": records})
			return
		}
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "GET, POST")
			return
		}
		defer r.Body.Close()
		var req struct {
			AsOf           string   `json:"as_of"`
			UniverseSource string   `json:"universe_source"`
			Symbols        []string `json:"symbols"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, (16<<10)+1))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "invalid scan request", http.StatusBadRequest)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			http.Error(w, "invalid scan request", http.StatusBadRequest)
			return
		}
		if req.UniverseSource != "USER_SUPPLIED" || len(req.Symbols) == 0 || len(req.Symbols) > 20 {
			http.Error(w, "invalid scan inputs", http.StatusBadRequest)
			return
		}
		result, err := h.scans.Submit(r.Context(), req.AsOf, req.UniverseSource, req.Symbols)
		if err != nil {
			if strings.Contains(err.Error(), "already running") {
				http.Error(w, "scan already running", http.StatusConflict)
				return
			}
			http.Error(w, "scan rejected", http.StatusBadRequest)
			return
		}
		sendJSON(w, http.StatusAccepted, result)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/scans/")
	parts := strings.Split(id, "/")
	// GET /results/{symbol}/snapshot returns the exact observed bars and PIT
	// factors behind one symbol's verdict for offline re-verification.
	if len(parts) == 4 && parts[1] == "results" && parts[3] == "snapshot" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		if !isRunID(parts[0]) {
			http.NotFound(w, r)
			return
		}
		h.scanSnapshot(w, r, parts[0], parts[2])
		return
	}
	// GET /comparison returns the deterministic same-as-of ranking over the
	// scan's persisted observations; source digests stay anchored per row.
	if len(parts) == 2 && parts[1] == "comparison" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		if !isRunID(parts[0]) {
			http.NotFound(w, r)
			return
		}
		h.scanComparison(w, r, parts[0])
		return
	}
	// GET /replay re-derives the persisted results from the persisted inputs.
	if len(parts) == 2 && parts[1] == "replay" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		if !isRunID(parts[0]) {
			http.NotFound(w, r)
			return
		}
		h.scanReplay(w, r, parts[0])
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	if !isRunID(id) {
		http.NotFound(w, r)
		return
	}
	result, err := h.scans.Get(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "scan store unavailable", http.StatusInternalServerError)
		return
	}
	sendJSON(w, http.StatusOK, result)
}

// scanSnapshot serves one persisted input observation. Unfinished scans have
// no committed snapshot surface; symbols without a valid observation report
// their INSUFFICIENT_DATA verdict instead of a fabricated empty series.
func (h *Handler) scanSnapshot(w http.ResponseWriter, r *http.Request, scanID, symbol string) {
	if !isRunID(scanID) {
		http.NotFound(w, r)
		return
	}
	obs, err := h.scans.GetSnapshot(scanID, symbol)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, application.ErrScanSnapshotNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, application.ErrScanNotCompleted) {
			http.Error(w, "scan snapshots require a completed scan", http.StatusConflict)
			return
		}
		http.Error(w, "scan snapshot unavailable", http.StatusInternalServerError)
		return
	}
	sendJSON(w, http.StatusOK, obs)
}

// scanComparison serves the deterministic cross-symbol ranking. Digest or
// store failures fail closed instead of serving unverifiable rankings.
func (h *Handler) scanComparison(w http.ResponseWriter, r *http.Request, scanID string) {
	comparison, err := h.scans.Comparison(scanID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, application.ErrScanNotCompleted) {
			http.Error(w, "scan comparison requires a completed scan", http.StatusConflict)
			return
		}
		http.Error(w, "scan comparison unavailable", http.StatusInternalServerError)
		return
	}
	sendJSON(w, http.StatusOK, comparison)
}

// scanReplay serves the replay verdict over persisted inputs. A tampered
// record reports matches=false rather than a fabricated reproduction.
func (h *Handler) scanReplay(w http.ResponseWriter, r *http.Request, scanID string) {
	replay, err := h.scans.Replay(scanID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, application.ErrScanNotCompleted) {
			http.Error(w, "scan replay requires a completed scan", http.StatusConflict)
			return
		}
		http.Error(w, "scan replay unavailable", http.StatusInternalServerError)
		return
	}
	sendJSON(w, http.StatusOK, replay)
}
