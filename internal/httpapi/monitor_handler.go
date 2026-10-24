package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"Aegis/internal/application"
)

// MonitorController exposes watch-condition rules and their recorded
// outcomes. It never triggers detection synchronously; the scheduler's
// MONITOR tasks own every pass.
type MonitorController interface {
	CreateRule(rule application.MonitorRule) (application.MonitorRule, error)
	UpdateRule(rule application.MonitorRule) error
	ListRules() ([]application.MonitorRule, error)
	GetRule(id string) (application.MonitorRule, error)
	DeleteRule(id string) error
	ListEvents(ruleID string) ([]application.MonitorEvent, error)
}

func (h *Handler) WithMonitorController(mc MonitorController) *Handler { h.monitors = mc; return h }

func (h *Handler) monitorRoute(w http.ResponseWriter, r *http.Request) {
	if h.monitors == nil {
		http.Error(w, "monitor unavailable", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/monitors")
	if path == "" || path == "/" {
		switch r.Method {
		case http.MethodGet:
			rules, err := h.monitors.ListRules()
			if err != nil {
				http.Error(w, "monitor store unavailable", http.StatusInternalServerError)
				return
			}
			sendJSON(w, http.StatusOK, map[string]any{"rules": rules})
		case http.MethodPost:
			h.monitorCreate(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	id := parts[0]
	if len(id) != 16 || !isHexID(id) {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "events" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		events, err := h.monitors.ListEvents(id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.Error(w, "invalid monitor rule", http.StatusNotFound)
				return
			}
			http.Error(w, "monitor store unavailable", http.StatusInternalServerError)
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"events": events})
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		rule, err := h.monitors.GetRule(id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "monitor store unavailable", http.StatusInternalServerError)
			return
		}
		sendJSON(w, http.StatusOK, rule)
	case http.MethodDelete:
		if err := h.monitors.DeleteRule(id); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "monitor store unavailable", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, "GET, DELETE")
	}
}

func (h *Handler) monitorCreate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req struct {
		Symbol       string                  `json:"symbol"`
		Kind         application.MonitorKind `json:"kind"`
		Threshold    float64                 `json:"threshold"`
		CooldownDays int                     `json:"cooldown_days"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, (8<<10)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid monitor request", http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid monitor request", http.StatusBadRequest)
		return
	}
	id, err := application.NewScheduleTaskID()
	if err != nil {
		http.Error(w, "monitor store unavailable", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	rule := application.MonitorRule{
		ID: id, Symbol: strings.ToUpper(strings.TrimSpace(req.Symbol)), Kind: req.Kind,
		Threshold: req.Threshold, CooldownDays: req.CooldownDays, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	created, err := h.monitors.CreateRule(rule)
	if err != nil {
		http.Error(w, "invalid monitor rule", http.StatusBadRequest)
		return
	}
	sendJSON(w, http.StatusCreated, created)
}
