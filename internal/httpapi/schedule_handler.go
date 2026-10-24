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

// ScheduleController exposes persisted schedule tasks and their execution
// history. It never triggers anything synchronously; the scheduler owns all
// triggering decisions.
type ScheduleController interface {
	CreateTask(task application.ScheduleTask) (application.ScheduleTask, error)
	ListTasks() ([]application.ScheduleTask, error)
	GetTask(id string) (application.ScheduleTask, error)
	UpdateTask(task application.ScheduleTask) error
	DeleteTask(id string) error
	ListRuns(taskID string) ([]application.ScheduleRun, error)
}

func (h *Handler) WithScheduleController(sc ScheduleController) *Handler { h.schedules = sc; return h }

func (h *Handler) scheduleRoute(w http.ResponseWriter, r *http.Request) {
	if h.schedules == nil {
		http.Error(w, "scheduler unavailable", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/schedules")
	if path == "" || path == "/" {
		switch r.Method {
		case http.MethodGet:
			tasks, err := h.schedules.ListTasks()
			if err != nil {
				http.Error(w, "schedule store unavailable", http.StatusInternalServerError)
				return
			}
			sendJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
		case http.MethodPost:
			h.scheduleCreate(w, r)
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
	if len(parts) == 2 && parts[1] == "runs" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		runs, err := h.schedules.ListRuns(id)
		if err != nil {
			http.Error(w, "schedule store unavailable", http.StatusInternalServerError)
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"runs": runs})
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		task, err := h.schedules.GetTask(id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "schedule store unavailable", http.StatusInternalServerError)
			return
		}
		sendJSON(w, http.StatusOK, task)
	case http.MethodDelete:
		if err := h.schedules.DeleteTask(id); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "schedule store unavailable", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, "GET, DELETE")
	}
}

func (h *Handler) scheduleCreate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req struct {
		Name           string                   `json:"name"`
		Kind           application.ScheduleKind `json:"kind"`
		Symbol         string                   `json:"symbol"`
		UniverseSource string                   `json:"universe_source"`
		Symbols        []string                 `json:"symbols"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, (16<<10)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid schedule request", http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid schedule request", http.StatusBadRequest)
		return
	}
	id, err := application.NewScheduleTaskID()
	if err != nil {
		http.Error(w, "schedule store unavailable", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	task := application.ScheduleTask{
		ID: id, Name: strings.TrimSpace(req.Name), Kind: req.Kind, Symbol: strings.ToUpper(strings.TrimSpace(req.Symbol)),
		UniverseSource: req.UniverseSource, Symbols: req.Symbols, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	created, err := h.schedules.CreateTask(task)
	if err != nil {
		http.Error(w, "invalid schedule task", http.StatusBadRequest)
		return
	}
	sendJSON(w, http.StatusCreated, created)
}

func isHexID(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
