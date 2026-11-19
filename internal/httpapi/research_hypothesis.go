package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"Aegis/internal/application"
)

func (h *Handler) WithHypothesisStore(store *application.FileHypothesisStore) *Handler {
	h.hypotheses = store
	return h
}

func (h *Handler) hypothesisRoute(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		methodNotAllowed(w, "GET, POST")
		return
	}
	if h.hypotheses == nil {
		http.Error(w, "hypothesis store unavailable", http.StatusServiceUnavailable)
		return
	}
	run, ok := h.load(w, r, runID)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		items, err := h.hypotheses.List(run.ID)
		if err != nil {
			http.Error(w, "hypothesis store unavailable", http.StatusInternalServerError)
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"hypotheses": items})
		return
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	var draft application.HypothesisDraft
	if err := decoder.Decode(&draft); err != nil {
		http.Error(w, "invalid hypothesis request", http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "invalid hypothesis request", http.StatusBadRequest)
		return
	}
	item, created, err := h.hypotheses.Add(run, draft, h.now())
	switch {
	case errors.Is(err, application.ErrInvalidHypothesis):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, application.ErrHypothesisCapacity), errors.Is(err, application.ErrHypothesisSource):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, "hypothesis store unavailable", http.StatusInternalServerError)
	case created:
		sendJSON(w, http.StatusCreated, item)
	default:
		sendJSON(w, http.StatusOK, item)
	}
}

func (h *Handler) hypothesisObservationRoute(w http.ResponseWriter, r *http.Request, runID, hypothesisID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		methodNotAllowed(w, "GET, POST")
		return
	}
	if h.hypotheses == nil {
		http.Error(w, "hypothesis store unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		records, err := h.hypotheses.Observations(runID, hypothesisID)
		if err != nil {
			hypothesisObservationError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"observations": records})
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	var input struct {
		RunID string `json:"run_id"`
	}
	var extra any
	if decoder.Decode(&input) != nil || !isRunID(input.RunID) || decoder.Decode(&extra) != io.EOF {
		http.Error(w, "invalid observation request", http.StatusBadRequest)
		return
	}
	run, ok := h.load(w, r, input.RunID)
	if !ok {
		return
	}
	record, created, err := h.hypotheses.Observe(runID, hypothesisID, run)
	if err != nil {
		hypothesisObservationError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	sendJSON(w, status, record)
}

func hypothesisObservationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrHypothesisNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, application.ErrObservationSource), errors.Is(err, application.ErrHypothesisCapacity):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, "hypothesis store unavailable", http.StatusInternalServerError)
	}
}

func (h *Handler) hypothesisReviewRoute(w http.ResponseWriter, r *http.Request, runID, hypothesisID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		methodNotAllowed(w, "GET, POST")
		return
	}
	if h.hypotheses == nil {
		http.Error(w, "hypothesis store unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		reviews, err := h.hypotheses.Reviews(runID, hypothesisID)
		if err != nil {
			hypothesisObservationError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"reviews": reviews})
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	var draft application.HypothesisReviewDraft
	var extra any
	if decoder.Decode(&draft) != nil || decoder.Decode(&extra) != io.EOF {
		http.Error(w, "invalid review request", http.StatusBadRequest)
		return
	}
	review, created, err := h.hypotheses.AddReview(runID, hypothesisID, draft, h.now())
	switch {
	case errors.Is(err, application.ErrInvalidHypothesisReview):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, application.ErrReviewObservationMissing):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		hypothesisObservationError(w, err)
	case created:
		sendJSON(w, http.StatusCreated, review)
	default:
		sendJSON(w, http.StatusOK, review)
	}
}
