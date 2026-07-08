package manager

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/store"
)

// An unknown session must fail on the session lookup, not be reported as a
// session that merely has no turns yet.
func TestGetLatestTurnUnknownSessionIsNotErrNoTurns(t *testing.T) {
	m, _ := newMgr(t)

	_, err := m.GetLatestTurn(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("GetLatestTurn on an unknown session returned nil error")
	}
	if errors.Is(err, store.ErrNoTurns) {
		t.Errorf("unknown session reported as ErrNoTurns: %v", err)
	}
}

func TestGetLatestTurnKnownSessionWithNoTurns(t *testing.T) {
	m, _ := newMgr(t)
	ctx := context.Background()

	sess, err := m.CreateSession(ctx, "demo", "rein")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.GetLatestTurn(ctx, sess.ID)
	if !errors.Is(err, store.ErrNoTurns) {
		t.Fatalf("err = %v, want ErrNoTurns", err)
	}
}
