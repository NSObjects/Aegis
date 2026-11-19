package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"Aegis/internal/settings"
)

func (h *Handler) WithPortfolioAccounts(store *settings.PortfolioAccountStore) *Handler {
	h.portfolioAccounts = store
	return h
}

func (h *Handler) portfolioAccountVersions(w http.ResponseWriter, r *http.Request) {
	if h.localDev || h.portfolioAccounts == nil {
		http.Error(w, "authenticated portfolio storage unavailable", http.StatusServiceUnavailable)
		return
	}
	now := h.now()
	switch r.Method {
	case http.MethodPost:
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
		decoder.DisallowUnknownFields()
		var input settings.AccountImport
		var extra any
		if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF {
			http.Error(w, "invalid portfolio import", http.StatusBadRequest)
			return
		}
		view, created, err := h.portfolioAccounts.Import(input, h.now())
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		sendAccountVersion(w, status, view, err)
	case http.MethodGet:
		query := r.URL.Query()
		for key, values := range query {
			switch key {
			case "account_id", "snapshot_version", "policy_version", "as_of":
			default:
				http.Error(w, "invalid portfolio query", http.StatusBadRequest)
				return
			}
			if len(values) != 1 || len(values[0]) > 128 || strings.TrimSpace(values[0]) != values[0] {
				http.Error(w, "invalid portfolio query", http.StatusBadRequest)
				return
			}
		}
		id, version := query.Get("account_id"), query.Get("snapshot_version")
		if id == "" || version == "" {
			http.Error(w, "account_id and snapshot_version are required", http.StatusBadRequest)
			return
		}
		cutoff := now
		if query.Has("as_of") {
			var err error
			cutoff, err = time.Parse(time.RFC3339, query.Get("as_of"))
			if err != nil || cutoff.IsZero() || cutoff.After(now) {
				http.Error(w, "invalid research cutoff", http.StatusBadRequest)
				return
			}
		}
		view, err := h.portfolioAccounts.Get(id, version, query.Get("policy_version"), cutoff)
		sendAccountVersion(w, http.StatusOK, view, err)
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func sendAccountVersion(w http.ResponseWriter, status int, view any, err error) {
	switch {
	case errors.Is(err, settings.ErrAccountImportInvalid):
		http.Error(w, settings.ErrAccountImportInvalid.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, settings.ErrAccountVersionMissing):
		http.Error(w, settings.ErrAccountVersionMissing.Error(), http.StatusNotFound)
	case errors.Is(err, settings.ErrAccountVersionConflict), errors.Is(err, settings.ErrAccountStoreCapacity):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, "portfolio storage unavailable", http.StatusInternalServerError)
	default:
		sendJSON(w, status, view)
	}
}
