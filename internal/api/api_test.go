package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/model"
)

type fakeSvc struct {
	sess model.Session
	turn model.Turn
}

func (f *fakeSvc) CreateSession(_ context.Context, label, owner string) (model.Session, error) {
	f.sess = model.Session{ID: uuid.New(), Label: label, Owner: owner, Status: model.SessionActive}
	return f.sess, nil
}
func (f *fakeSvc) ListSessions(context.Context, string) ([]model.Session, error) {
	return []model.Session{f.sess}, nil
}
func (f *fakeSvc) GetSession(_ context.Context, id uuid.UUID) (model.Session, error) {
	return f.sess, nil
}
func (f *fakeSvc) GetTurn(_ context.Context, id uuid.UUID) (model.Turn, error) { return f.turn, nil }
func (f *fakeSvc) EnqueueTurn(_ context.Context, sid uuid.UUID, prompt string) (model.Turn, error) {
	f.turn = model.Turn{ID: uuid.New(), SessionID: sid, Prompt: prompt, Status: model.TurnQueued}
	return f.turn, nil
}
func (f *fakeSvc) Subscribe(uuid.UUID) (<-chan model.Event, func()) {
	ch := make(chan model.Event)
	close(ch)
	return ch, func() {}
}
func (f *fakeSvc) Summarize(context.Context, uuid.UUID, bool) (model.Session, string, error) {
	return f.sess, "summary text", nil
}
func (f *fakeSvc) Archive(context.Context, uuid.UUID) error { return nil }

func TestCreateSessionEndpoint(t *testing.T) {
	r := NewRouter(&fakeSvc{})
	req := httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(`{"label":"demo","owner":"rein"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body)
	}
	var got model.Session
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Label != "demo" {
		t.Errorf("label = %q", got.Label)
	}
}

func TestEnqueueTurnReturns202(t *testing.T) {
	r := NewRouter(&fakeSvc{})
	id := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/sessions/"+id+"/messages", strings.NewReader(`{"prompt":"hi"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", w.Code, w.Body)
	}
	var got model.Turn
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Status != model.TurnQueued {
		t.Errorf("status = %q", got.Status)
	}
}

func TestHealthLive(t *testing.T) {
	r := NewRouter(&fakeSvc{})
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}
