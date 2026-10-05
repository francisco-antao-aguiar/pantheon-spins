package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"pantheon-spins/server/internal/api"
)

const maxBodyBytes = 64 << 10

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, api.Error{Code: code, Message: msg})
}

// decodeJSON reads a size-limited JSON body into dst, rejecting unknown fields.
// On failure it writes a 400 and returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON for this endpoint.")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be a single JSON object.")
		return false
	}
	return true
}
