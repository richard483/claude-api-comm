package manager

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/richard483/claude-api-comm/internal/broker"
	"github.com/richard483/claude-api-comm/internal/model"
	"github.com/richard483/claude-api-comm/internal/runner"
	"github.com/richard483/claude-api-comm/internal/settings"
	"github.com/richard483/claude-api-comm/internal/store"
)

// wsCheckingExec mirrors ClaudeExecutor's contract: it cannot run without a
// real working directory.
type wsCheckingExec struct {
	mu           sync.Mutex
	sawWorkspace string
}

func (f *wsCheckingExec) Run(_ context.Context, workspace, _, _ string, emit func(model.Event)) (model.Result, error) {
	fi, err := os.Stat(workspace)
	if err != nil {
		return model.Result{}, fmt.Errorf("workspace unavailable: %s: %w", workspace, err)
	}
	if !fi.IsDir() {
		return model.Result{}, fmt.Errorf("workspace is not a directory: %s", workspace)
	}
	f.mu.Lock()
	f.sawWorkspace = workspace
	f.mu.Unlock()
	emit(model.Event{Type: "text", Text: "hi"})
	return model.Result{ClaudeSessionID: "claude-xyz", Text: "answer", NumTurns: 1}, nil
}

func newMgrWithWSCheck(t *testing.T) (*Manager, *wsCheckingExec) {
	t.Helper()
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
	fx := &wsCheckingExec{}
	return New(st, &runner.WorktreeRunner{BaseDir: t.TempDir()}, fx, broker.New(), settings.New(""), 2, time.Minute), fx
}

// The idle reaper (or a reinstall) can delete a live session's workspace. The
// session row still points at it, and every later turn used to die inside
// exec with "fork/exec <claude bin>: no such file or directory". A turn must
// recreate the workspace instead.
func TestTurnRecreatesReapedWorkspace(t *testing.T) {
	m, fx := newMgrWithWSCheck(t)
	ctx := context.Background()

	sess, err := m.CreateSession(ctx, "demo", "rein")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(sess.WorkspacePath); err != nil { // the reaper strikes
		t.Fatal(err)
	}

	turn, err := m.EnqueueTurn(ctx, sess.ID, "still there?")
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var got model.Turn
	for time.Now().Before(deadline) {
		got, _ = m.GetTurn(ctx, turn.ID)
		if got.Status.Terminal() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got.Status != model.TurnDone {
		t.Fatalf("turn should have survived a reaped workspace, got status=%q err=%q", got.Status, got.Error)
	}
	if fi, err := os.Stat(sess.WorkspacePath); err != nil || !fi.IsDir() {
		t.Fatalf("workspace not recreated at %s: %v", sess.WorkspacePath, err)
	}
	fx.mu.Lock()
	defer fx.mu.Unlock()
	if fx.sawWorkspace != sess.WorkspacePath {
		t.Errorf("executor ran in %q, want the session workspace %q", fx.sawWorkspace, sess.WorkspacePath)
	}
}

// Summarize runs claude in the same workspace and must self-heal identically.
func TestSummarizeRecreatesReapedWorkspace(t *testing.T) {
	m, _ := newMgrWithWSCheck(t)
	ctx := context.Background()

	sess, err := m.CreateSession(ctx, "demo", "rein")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(sess.WorkspacePath); err != nil {
		t.Fatal(err)
	}

	if _, _, err := m.Summarize(ctx, sess.ID, false); err != nil {
		t.Fatalf("Summarize on a reaped workspace: %v", err)
	}
	if fi, err := os.Stat(sess.WorkspacePath); err != nil || !fi.IsDir() {
		t.Fatalf("workspace not recreated at %s: %v", sess.WorkspacePath, err)
	}
}
