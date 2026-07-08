package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/model"
)

func TestLatestTurnReturnsFlatBodyWithPending(t *testing.T) {
	svc := &fakeSvc{latest: model.LatestTurn{
		Turn:    model.Turn{ID: uuid.New(), Prompt: "hello", Result: "hi there", Status: model.TurnDone},
		Pending: true,
	}}
	r := NewRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+uuid.New().String()+"/turns/latest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["prompt"] != "hello" || got["result"] != "hi there" {
		t.Errorf("prompt/result not at top level: %v", got)
	}
	if got["status"] != "done" {
		t.Errorf("status = %v", got["status"])
	}
	if got["pending"] != true {
		t.Errorf("pending = %v, want true", got["pending"])
	}
}

func TestLatestTurnRejectsInvalidSessionID(t *testing.T) {
	r := NewRouter(&fakeSvc{})

	req := httptest.NewRequest(http.MethodGet, "/sessions/not-a-uuid/turns/latest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body)
	}
	// Assert on the message, not just the code. Before the route exists this
	// path falls through to getTurn, which also answers 400 — but with
	// "invalid turn id". Without this check the test would pass vacuously.
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] != "invalid id" {
		t.Errorf("error = %q, want %q (request reached getTurn, not latestTurn)", got["error"], "invalid id")
	}
}

func TestLatestTurnReturns404WhenSessionHasNoTurns(t *testing.T) {
	svc := &fakeSvc{latestErr: errors.New("no turns for session")}
	r := NewRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+uuid.New().String()+"/turns/latest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body)
	}
}

// "latest" must not be parsed as a turn id by the /turns/{turnID} route.
func TestLatestTurnRouteIsNotSwallowedByGetTurn(t *testing.T) {
	svc := &fakeSvc{
		turn:   model.Turn{Prompt: "WRONG-came-from-getTurn"},
		latest: model.LatestTurn{Turn: model.Turn{Prompt: "RIGHT-came-from-latestTurn"}},
	}
	r := NewRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+uuid.New().String()+"/turns/latest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body)
	}
	var got model.LatestTurn
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Prompt != "RIGHT-came-from-latestTurn" {
		t.Errorf("prompt = %q; the request reached getTurn instead of latestTurn", got.Prompt)
	}
}
