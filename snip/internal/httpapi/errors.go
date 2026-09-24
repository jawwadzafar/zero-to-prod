package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jawwadzafar/zero-to-prod/snip/internal/auth"
	"github.com/jawwadzafar/zero-to-prod/snip/internal/links"
)

// errorBody is the one shape every error response has, so clients can
// handle errors with a single piece of code:
//
//	{"error": {"code": "invalid_url", "message": "url must be an absolute http(s) URL"}}
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var b errorBody
	b.Error.Code = code
	b.Error.Message = message
	writeJSON(w, status, b)
}

// writeDomainError maps domain errors to HTTP. Anything unexpected becomes a
// 500 with a generic message: internal details go to the log, never to the
// client (they can leak information to attackers).
func (s *Server) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, links.ErrInvalidURL):
		writeError(w, http.StatusBadRequest, "invalid_url", "url must be an absolute http(s) URL of at most 2048 characters")
	case errors.Is(err, links.ErrInvalidSlug):
		writeError(w, http.StatusBadRequest, "invalid_slug", "slug must be 3-32 letters, digits, '-' or '_', and not a reserved word")
	case errors.Is(err, links.ErrSlugTaken):
		writeError(w, http.StatusConflict, "slug_taken", "that slug is already in use")
	case errors.Is(err, links.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such link")
	case errors.Is(err, auth.ErrUnknownKey):
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid API key")
	default:
		s.log.ErrorContext(r.Context(), "internal error", "err", err, "path", r.URL.Path)
		writeError(w, http.StatusInternalServerError, "internal", "something went wrong on our side")
	}
}
