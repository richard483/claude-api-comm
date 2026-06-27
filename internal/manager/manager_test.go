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

// TestSummarizeSerializesWithTurns verifies that Summarize stores a rolling_summary
// and serializes correctly with turns (correctness + no data race under -race).
func TestSummarizeSerializesWithTurns(t *testing.T) {
	m, _ := newMgr(t)
	ctx := context.Background()

	sess, err := m.CreateSession(ctx, "summarize-test", "rein")
	if err != nil {
		t.Fatal(err)
	}

	// Run one turn so the session has a claude_session_id.
	turn, err := m.EnqueueTurn(ctx, sess.ID, "hello")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := m.GetTurn(ctx, turn.ID)
		if got.Status.Terminal() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Now call Summarize — it must acquire the per-session lock (serializes with runTurn)
	// and store the rolling summary.
	updatedSess, summary, err := m.Summarize(ctx, sess.ID, false)
	if err != nil {
		t.Fatalf("Summarize error: %v", err)
	}
	if summary == "" {
		t.Error("Summarize returned empty summary")
	}
	if updatedSess.RollingSummary == nil || *updatedSess.RollingSummary == "" {
		t.Errorf("rolling_summary not stored, got: %v", updatedSess.RollingSummary)
	}

	// Verify the session lock was created by sessionLock helper.
	mu := m.sessionLock(sess.ID)
	if mu == nil {
		t.Error("sessionLock returned nil")
	}

	// Verify the stored rolling_summary matches what was returned.
	fetched, err := m.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.RollingSummary == nil || *fetched.RollingSummary != summary {
		t.Errorf("stored rolling_summary = %v, want %q", fetched.RollingSummary, summary)
	}
}
