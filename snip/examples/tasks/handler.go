package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
)

// --- Handler layer (the "controller"): HTTP in, HTTP out. -------------------
// It decodes requests, calls the service, and encodes responses. It holds no
// business rules of its own.

type Handler struct {
	svc *Service
	log *slog.Logger
}

// Routes is the router: method + path pattern → handler.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tasks", h.create)
	mux.HandleFunc("GET /tasks", h.list)
	mux.HandleFunc("GET /tasks/{id}", h.get)
	return mux
}

type createTaskRequest struct {
	Title string `json:"title"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	// Layers 1 and 2 of validation (size and syntax) belong here: they are
	// about the HTTP request, not about tasks.
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)) // at most 4 KiB
	dec.DisallowUnknownFields()                                  // {"titel": …} is an error, not silently ignored
	var req createTaskRequest
	if err := dec.Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body must be at most 4 KiB")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_json", `body must be JSON like {"title": "buy milk"}`)
		return
	}
	t, err := h.svc.Create(r.Context(), req.Title) // the request's context travels down
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Location", "/tasks/"+strconv.FormatInt(t.ID, 10))
	writeJSON(w, http.StatusCreated, t)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64) // the {id} part of the path
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid_id", "id must be a positive whole number")
		return
	}
	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit")) // ?limit=10; missing or junk → 0 → default
	ts, err := h.svc.List(r.Context(), limit)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": ts}) // an object, so fields can be added later
}

// writeDomainError is the ONE place that turns errors into HTTP answers.
func (h *Handler) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidTitle):
		writeError(w, http.StatusBadRequest, "invalid_title", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such task")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusServiceUnavailable, "timeout", "the request took too long; try again")
	case errors.Is(err, context.Canceled):
		h.log.InfoContext(r.Context(), "client went away", "request_id", RequestID(r.Context()))
	default: // anything unexpected: details to the log, nothing internal to the client
		h.log.ErrorContext(r.Context(), "internal error", "err", err, "request_id", RequestID(r.Context()))
		writeError(w, http.StatusInternalServerError, "internal", "something went wrong on our side")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Every error has the same shape: {"error": {"code": "...", "message": "..."}}.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
