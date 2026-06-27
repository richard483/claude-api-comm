package manager

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/richard483/claude-api-comm/internal/broker"
	"github.com/richard483/claude-api-comm/internal/model"
	"github.com/richard483/claude-api-comm/internal/runner"
	"github.com/richard483/claude-api-comm/internal/store"
)

type fakeExec struct {
	mu        sync.Mutex
	gotResume string
	gotPrompt string
}

func (f *fakeExec) Run(_ context.Context, _, prompt, resumeID string, emit func(model.Event)) (model.Result, error) {
	f.mu.Lock()
	f.gotResume = resumeID
	f.gotPrompt = prompt
	f.mu.Unlock()
	emit(model.Event{Type: "text", Text: "hi"})
	return model.Result{ClaudeSessionID: "claude-xyz", Text: "answer", NumTurns: 1}, nil
}

func (f *fakeExec) resume() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gotResume
}

func newMgr(t *testing.T) (*Manager, *fakeExec) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := store.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	fx := &fakeExec{}
	m := New(st, &runner.WorktreeRunner{BaseDir: t.TempDir()}, fx, broker.New(), 2, time.Minute)
	return m, fx
}

func TestEnqueueRunsTurnAndCapturesClaudeID(t *testing.T) {
	m, fx := newMgr(t)
	ctx := context.Background()
	sess, err := m.CreateSession(ctx, "demo", "rein")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := m.EnqueueTurn(ctx, sess.ID, "what is 2+2")
	if err != nil {
		t.Fatal(err)
	}

	// poll until terminal
	deadline := time.Now().Add(5 * time.Second)
	var got model.Turn
	for time.Now().Before(deadline) {
		got, _ = m.GetTurn(ctx, turn.ID)
		if got.Status.Terminal() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got.Status != model.TurnDone || got.Result != "answer" {
		t.Fatalf("turn not done: %+v", got)
	}
	if fx.resume() != "" {
		t.Errorf("first turn should not resume, got %q", fx.resume())
	}
	sess2, _ := m.GetSession(ctx, sess.ID)
	if sess2.ClaudeSessionID == nil || *sess2.ClaudeSessionID != "claude-xyz" {
		t.Errorf("claude id not captured: %+v", sess2.ClaudeSessionID)
	}

	// second turn must resume with the captured id
	turn2, _ := m.EnqueueTurn(ctx, sess.ID, "again")
	deadline2 := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline2) {
		g, _ := m.GetTurn(ctx, turn2.ID)
		if g.Status.Terminal() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if fx.resume() != "claude-xyz" {
		t.Errorf("second turn resume = %q, want claude-xyz", fx.resume())
	}
}
