package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/model"
)

func seedSession(t *testing.T, s *Store) model.Session {
	t.Helper()
	sess, err := s.CreateSession(context.Background(), model.Session{
		ID: uuid.New(), WorkspacePath: t.TempDir(), Backend: "worktree",
		Label: "t", Owner: "t", Status: model.SessionActive,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return sess
}

// seedTurn inserts a turn and drives it to the requested status.
// It sleeps 1ms so successive turns get strictly increasing created_at values,
// which the ORDER BY in GetLatestTurn relies on.
func seedTurn(t *testing.T, s *Store, sessionID uuid.UUID, prompt string, status model.TurnStatus, result, errText string) model.Turn {
	t.Helper()
	ctx := context.Background()
	turn, err := s.CreateTurn(ctx, model.Turn{
		ID: uuid.New(), SessionID: sessionID, Prompt: prompt, Status: model.TurnQueued,
	})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	switch status {
	case model.TurnQueued:
		// already queued from CreateTurn above; no state transition needed.
	case model.TurnRunning:
		if err := s.StartTurn(ctx, turn.ID); err != nil {
			t.Fatalf("StartTurn: %v", err)
		}
	case model.TurnDone, model.TurnError:
		turn.Status = status
		turn.Result = result
		turn.Error = errText
		if err := s.FinishTurn(ctx, turn); err != nil {
			t.Fatalf("FinishTurn: %v", err)
		}
	}
	turn.Status = status
	time.Sleep(time.Millisecond)
	return turn
}

func TestGetLatestTurnNoTurnsReturnsErrNoTurns(t *testing.T) {
	s := newTestStore(t)
	sess := seedSession(t, s)

	_, err := s.GetLatestTurn(context.Background(), sess.ID)
	if !errors.Is(err, ErrNoTurns) {
		t.Fatalf("err = %v, want ErrNoTurns", err)
	}
}

func TestGetLatestTurnFallsBackToRunningTurn(t *testing.T) {
	s := newTestStore(t)
	sess := seedSession(t, s)
	running := seedTurn(t, s, sess.ID, "what is 2+2", model.TurnRunning, "", "")

	got, err := s.GetLatestTurn(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("GetLatestTurn: %v", err)
	}
	if got.ID != running.ID {
		t.Errorf("id = %v, want the running turn %v", got.ID, running.ID)
	}
	if got.Status != model.TurnRunning || got.Result != "" {
		t.Errorf("got status=%q result=%q, want running with empty result", got.Status, got.Result)
	}
	if !got.Pending {
		t.Error("pending = false, want true while a turn is running")
	}
}

func TestGetLatestTurnFallsBackToQueuedTurn(t *testing.T) {
	s := newTestStore(t)
	sess := seedSession(t, s)
	queued := seedTurn(t, s, sess.ID, "what is 2+2", model.TurnQueued, "", "")

	got, err := s.GetLatestTurn(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("GetLatestTurn: %v", err)
	}
	if got.ID != queued.ID {
		t.Errorf("id = %v, want the queued turn %v", got.ID, queued.ID)
	}
	if got.Status != model.TurnQueued || got.Result != "" {
		t.Errorf("got status=%q result=%q, want queued with empty result", got.Status, got.Result)
	}
	if !got.Pending {
		t.Error("pending = false, want true while a turn is queued")
	}
}

func TestGetLatestTurnPrefersFinishedTurnOverQueuedOne(t *testing.T) {
	s := newTestStore(t)
	sess := seedSession(t, s)
	done := seedTurn(t, s, sess.ID, "hello", model.TurnDone, "hi there", "")
	seedTurn(t, s, sess.ID, "what is 2+2", model.TurnQueued, "", "")

	got, err := s.GetLatestTurn(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("GetLatestTurn: %v", err)
	}
	if got.ID != done.ID {
		t.Errorf("id = %v, want the finished turn %v", got.ID, done.ID)
	}
	if got.Result != "hi there" {
		t.Errorf("result = %q, want the last answer", got.Result)
	}
	if !got.Pending {
		t.Error("pending = false, want true because a newer turn is queued")
	}
}

func TestGetLatestTurnPrefersFinishedTurnOverRunningOne(t *testing.T) {
	s := newTestStore(t)
	sess := seedSession(t, s)
	done := seedTurn(t, s, sess.ID, "hello", model.TurnDone, "hi there", "")
	seedTurn(t, s, sess.ID, "what is 2+2", model.TurnRunning, "", "")

	got, err := s.GetLatestTurn(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("GetLatestTurn: %v", err)
	}
	if got.ID != done.ID {
		t.Errorf("id = %v, want the finished turn %v", got.ID, done.ID)
	}
	if got.Result != "hi there" {
		t.Errorf("result = %q, want the last answer", got.Result)
	}
	if !got.Pending {
		t.Error("pending = false, want true because a newer turn is running")
	}
}

func TestGetLatestTurnReturnsErrorTurnRatherThanStaleSuccess(t *testing.T) {
	s := newTestStore(t)
	sess := seedSession(t, s)
	seedTurn(t, s, sess.ID, "hello", model.TurnDone, "hi there", "")
	failed := seedTurn(t, s, sess.ID, "what is 2+2", model.TurnError, "", "workspace unavailable: /home/x")

	got, err := s.GetLatestTurn(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("GetLatestTurn: %v", err)
	}
	if got.ID != failed.ID {
		t.Errorf("id = %v, want the failed turn %v (failures must not be hidden)", got.ID, failed.ID)
	}
	if got.Status != model.TurnError || got.Error != "workspace unavailable: /home/x" {
		t.Errorf("got status=%q error=%q", got.Status, got.Error)
	}
	if got.Pending {
		t.Error("pending = true, want false when nothing is in flight")
	}
}
