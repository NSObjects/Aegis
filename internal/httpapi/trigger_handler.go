package httpapi

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"Aegis/internal/application"
)

// TriggerController exposes the persisted trigger audit trail read-only:
// every routed research escalation is inspectable after the fact.
type TriggerController interface {
	List(limit int) ([]application.TriggerRecord, error)
	Get(id string) (application.TriggerRecord, error)
}

func (h *Handler) WithTriggerController(tc TriggerController) *Handler { h.triggers = tc; return h }

func (h *Handler) triggerRoute(w http.ResponseWriter, r *http.Request) {
	if h.triggers == nil {
		http.Error(w, "trigger audit unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	if r.URL.Path == "/api/v1/triggers" {
		limit := 20
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 100 {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			}
			limit = n
		}
		records, err := h.triggers.List(limit)
		if err != nil {
			http.Error(w, "trigger store unavailable", http.StatusInternalServerError)
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"triggers": records})
		return
	}
	id := r.URL.Path[len("/api/v1/triggers/"):]
	record, err := h.triggers.Get(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "invalid trigger id", http.StatusBadRequest)
		return
	}
	sendJSON(w, http.StatusOK, record)
}
