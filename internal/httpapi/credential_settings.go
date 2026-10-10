package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"Aegis/internal/settings"
)

// WithCredentialStore enables single-operator provider credential settings on
// the same Bearer-authenticated API. The store never exposes plaintext keys.
func (h *Handler) WithCredentialStore(store *settings.Store) *Handler {
	h.credentials = store
	return h
}
func (h *Handler) credentialSettings(w http.ResponseWriter, r *http.Request) {
	if h.credentials == nil {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		status, err := h.credentials.Status(r.Context())
		if err != nil {
			http.Error(w, "credential storage unavailable", http.StatusInternalServerError)
			return
		}
		sendJSON(w, http.StatusOK, status)
	case http.MethodPut:
		raw, err := io.ReadAll(io.LimitReader(r.Body, 16<<10+1))
		if err != nil || len(raw) > 16<<10 {
			http.Error(w, "invalid credential request", http.StatusRequestEntityTooLarge)
			return
		}
		var input struct {
			Changes map[string]string `json:"changes"`
			Clear   []string          `json:"clear"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&input); err != nil {
			http.Error(w, "invalid credential request", http.StatusBadRequest)
			return
		}
		var rest any
		if dec.Decode(&rest) != io.EOF {
			http.Error(w, "invalid credential request", http.StatusBadRequest)
			return
		}
		status, err := h.credentials.Update(r.Context(), input.Changes, input.Clear)
		if errors.Is(err, settings.ErrReadOnly) {
			http.Error(w, "credential storage not enabled", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "invalid credential update or storage unavailable", http.StatusBadRequest)
			return
		}
		sendJSON(w, http.StatusOK, status)
	default:
		methodNotAllowed(w, "GET, PUT")
	}
}
