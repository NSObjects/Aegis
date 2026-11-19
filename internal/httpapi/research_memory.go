package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"Aegis/internal/application"
	"Aegis/internal/strategy/pullback"
)

func (h *Handler) researchMemoryRoute(w http.ResponseWriter, r *http.Request) {
	const path = "/api/v1/research/memory"
	if h.hypotheses == nil {
		http.Error(w, "research memory unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.URL.Path != path {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, path+"/"), "/")
		if len(parts) != 2 || !isRunID(parts[0]) || (parts[1] != "revoke" && parts[1] != "feedback") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		if parts[1] == "feedback" {
			h.addResearchFeedback(w, r, parts[0])
			return
		}
		entry, err := h.hypotheses.RevokeMemory(parts[0], h.now())
		sendMemoryResult(w, http.StatusOK, entry, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		offset := 0
		if q.Has("offset") {
			var err error
			offset, err = strconv.Atoi(q.Get("offset"))
			if err != nil {
				sendMemoryResult(w, 0, nil, application.ErrInvalidMemory)
				return
			}
		}
		results, err := h.hypotheses.SearchMemory(application.MemoryFilter{
			Symbol: q.Get("symbol"), Query: q.Get("q"), MarketPhase: q.Get("market_phase"),
			Category: q.Get("category"), Outcome: q.Get("outcome"), Offset: offset,
		})
		sendMemoryResult(w, http.StatusOK, results, err)
	case http.MethodPost:
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		var source application.MemorySource
		var extra any
		if decoder.Decode(&source) != nil || decoder.Decode(&extra) != io.EOF {
			sendMemoryResult(w, 0, nil, application.ErrInvalidMemory)
			return
		}
		entry, created, err := h.hypotheses.AddMemory(source, h.now())
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		sendMemoryResult(w, status, entry, err)
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func sendMemoryResult(w http.ResponseWriter, status int, body any, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidMemory):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, application.ErrHypothesisNotFound), errors.Is(err, application.ErrMemoryNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, application.ErrMemorySource), errors.Is(err, application.ErrMemoryCapacity),
		errors.Is(err, application.ErrMemoryRevoked), errors.Is(err, application.ErrFeedbackUnavailable):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, "research memory unavailable", http.StatusInternalServerError)
	default:
		sendJSON(w, status, body)
	}
}

func (h *Handler) addResearchFeedback(w http.ResponseWriter, r *http.Request, memoryID string) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20))
	decoder.DisallowUnknownFields()
	var input pullback.ValidationInput
	var extra any
	if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF {
		sendMemoryResult(w, 0, nil, application.ErrInvalidMemory)
		return
	}
	feedback, created, err := h.hypotheses.AddResearchFeedback(memoryID, input, h.now())
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	sendMemoryResult(w, status, feedback, err)
}
