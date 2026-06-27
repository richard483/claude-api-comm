package store

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/model"
)

func newTestStore(t *testing.T) *Store {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres-backed test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestSessionRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	in := model.Session{
		ID:            uuid.New(),
		WorkspacePath: "/tmp/ws",
		Backend:       "worktree",
		Label:         "demo",
		Owner:         "rein",
		Status:        model.SessionActive,
	}
	out, err := s.CreateSession(ctx, in)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if out.CreatedAt.IsZero() {
		t.Error("CreatedAt not set by DB")
	}
	got, err := s.GetSession(ctx, in.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Owner != "rein" || got.Backend != "worktree" {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if err := s.UpdateSessionClaudeID(ctx, in.ID, "claude-abc"); err != nil {
		t.Fatalf("UpdateSessionClaudeID: %v", err)
	}
	got, _ = s.GetSession(ctx, in.ID)
	if got.ClaudeSessionID == nil || *got.ClaudeSessionID != "claude-abc" {
		t.Errorf("claude id not persisted: %+v", got.ClaudeSessionID)
	}
}

func TestTurnLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sess, _ := s.CreateSession(ctx, model.Session{
		ID: uuid.New(), WorkspacePath: "/tmp/ws2", Backend: "worktree", Status: model.SessionActive,
	})
	turn, err := s.CreateTurn(ctx, model.Turn{
		ID: uuid.New(), SessionID: sess.ID, Prompt: "hi", Status: model.TurnQueued,
	})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if err := s.StartTurn(ctx, turn.ID); err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	turn.Status = model.TurnDone
	turn.Result = "hello"
	turn.NumTurns = 1
	turn.CostUSD = 0.01
	turn.DurationMs = 1234
	if err := s.FinishTurn(ctx, turn); err != nil {
		t.Fatalf("FinishTurn: %v", err)
	}
	got, _ := s.GetTurn(ctx, turn.ID)
	if got.Status != model.TurnDone || got.Result != "hello" || got.FinishedAt == nil {
		t.Errorf("finish not persisted: %+v", got)
	}
}
