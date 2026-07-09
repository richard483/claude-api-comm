package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/model"
	"github.com/richard483/claude-api-comm/internal/settings"
)

type fakeSvc struct {
	sess      model.Session
	turn      model.Turn
	latest    model.LatestTurn
	latestErr error
	cfg       model.Config
	setErr    error
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
func (f *fakeSvc) GetLatestTurn(context.Context, uuid.UUID) (model.LatestTurn, error) {
	return f.latest, f.latestErr
}
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
func (f *fakeSvc) Ready(context.Context) error              { return nil }
func (f *fakeSvc) GetConfig() model.Config                  { return f.cfg }
func (f *fakeSvc) SetModel(name string) (model.Config, error) {
	if f.setErr != nil {
		return model.Config{}, f.setErr
	}
	f.cfg = model.Config{Model: name}
	return f.cfg, nil
}
func (f *fakeSvc) ListModels() []settings.KnownModel { return settings.KnownModels }

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

func TestCreateSessionRejectsMalformedJSON(t *testing.T) {
	r := NewRouter(&fakeSvc{})
	req := httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body)
	}
}

func TestCreateSessionEmptyBodyOK(t *testing.T) {
	r := NewRouter(&fakeSvc{})
	req := httptest.NewRequest(http.MethodPost, "/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body)
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

func TestHealthReady(t *testing.T) {
	r := NewRouter(&fakeSvc{})
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body)
	}
}

func TestGetConfig(t *testing.T) {
	r := NewRouter(&fakeSvc{cfg: model.Config{Model: "claude-opus-4-8"}})
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body)
	}
	var got model.Config
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Model != "claude-opus-4-8" {
		t.Errorf("model = %q, want claude-opus-4-8", got.Model)
	}
}

func TestSetConfigUpdatesModel(t *testing.T) {
	r := NewRouter(&fakeSvc{})
	req := httptest.NewRequest(http.MethodPut, "/config", strings.NewReader(`{"model":"claude-sonnet-5"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body)
	}
	var got model.Config
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Model != "claude-sonnet-5" {
		t.Errorf("model = %q, want claude-sonnet-5", got.Model)
	}
}

func TestSetConfigRejectsUnknownModel(t *testing.T) {
	r := NewRouter(&fakeSvc{setErr: errors.New(`unknown model "gpt-5"`)})
	req := httptest.NewRequest(http.MethodPut, "/config", strings.NewReader(`{"model":"gpt-5"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body)
	}
}

func TestListModels(t *testing.T) {
	r := NewRouter(&fakeSvc{})
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body)
	}
	var got []settings.KnownModel
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one model")
	}
}
