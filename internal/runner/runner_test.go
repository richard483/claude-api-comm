package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestWorktreePrepareAndCleanup(t *testing.T) {
	base := t.TempDir()
	r := &WorktreeRunner{BaseDir: base}
	if r.Backend() != "worktree" {
		t.Errorf("Backend() = %q", r.Backend())
	}
	id := uuid.New()
	ws, err := r.Prepare(context.Background(), id)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if ws != filepath.Join(base, id.String()) {
		t.Errorf("ws = %q", ws)
	}
	if fi, err := os.Stat(ws); err != nil || !fi.IsDir() {
		t.Fatalf("workspace not created: %v", err)
	}
	if err := r.Cleanup(context.Background(), ws); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(ws); !os.IsNotExist(err) {
		t.Error("workspace not removed")
	}
}
