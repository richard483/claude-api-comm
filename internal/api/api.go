package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/model"
)

type Service interface {
	CreateSession(ctx context.Context, label, owner string) (model.Session, error)
	ListSessions(ctx context.Context, status string) ([]model.Session, error)
	GetSession(ctx context.Context, id uuid.UUID) (model.Session, error)
	GetTurn(ctx context.Context, id uuid.UUID) (model.Turn, error)
	EnqueueTurn(ctx context.Context, sessionID uuid.UUID, prompt string) (model.Turn, error)
	Subscribe(turnID uuid.UUID) (<-chan model.Event, func())
	Summarize(ctx context.Context, sessionID uuid.UUID, fork bool) (model.Session, string, error)
	Archive(ctx context.Context, sessionID uuid.UUID) error
}

type handler struct{ svc Service }

func NewRouter(svc Service) http.Handler {
	h := &handler{svc: svc}
	r := chi.NewRouter()

	// Auth middleware slot (none in v1):
	// r.Use(authMiddleware)

	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/health/ready", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	r.Post("/sessions", h.createSession)
	r.Get("/sessions", h.listSessions)
	r.Get("/sessions/{id}", h.getSession)
	r.Post("/sessions/{id}/messages", h.enqueueTurn)
	r.Get("/sessions/{id}/turns/{turnID}", h.getTurn)
	r.Get("/sessions/{id}/turns/{turnID}/stream", h.streamTurn)
	r.Post("/sessions/{id}/summarize", h.summarize)
	r.Post("/sessions/{id}/archive", h.archive)
	return r
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func pathUUID(r *http.Request, key string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, key))
}

func (h *handler) createSession(w http.ResponseWriter, r *http.Request) {
	var body struct{ Label, Owner string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		httpErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	s, err := h.svc.CreateSession(r.Context(), body.Label, body.Owner)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (h *handler) listSessions(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListSessions(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) getSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	s, err := h.svc.GetSession(r.Context(), id)
	if err != nil {
		httpErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *handler) enqueueTurn(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct{ Prompt string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Prompt == "" {
		httpErr(w, http.StatusBadRequest, "prompt required")
		return
	}
	t, err := h.svc.EnqueueTurn(r.Context(), id, body.Prompt)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, t)
}

func (h *handler) getTurn(w http.ResponseWriter, r *http.Request) {
	tid, err := pathUUID(r, "turnID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid turn id")
		return
	}
	t, err := h.svc.GetTurn(r.Context(), tid)
	if err != nil {
		httpErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *handler) streamTurn(w http.ResponseWriter, r *http.Request) {
	tid, err := pathUUID(r, "turnID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid turn id")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := h.svc.Subscribe(tid)
	defer cancel()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			flusher.Flush()
		}
	}
}

func (h *handler) summarize(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	fork := r.URL.Query().Get("fork") == "true"
	sess, summary, err := h.svc.Summarize(r.Context(), id, fork)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": sess, "summary": summary})
}

func (h *handler) archive(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Archive(r.Context(), id); err != nil {
		httpErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
