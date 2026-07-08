package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// Prepare only runs at session creation. The idle reaper (Cleanup) and manual
// housekeeping can remove a live session's workspace underneath it, so a turn
// must be able to recreate it.
func TestEnsureRecreatesReapedWorkspace(t *testing.T) {
	base := t.TempDir()
	r := &WorktreeRunner{BaseDir: base}
	ctx := context.Background()

	ws, err := r.Prepare(ctx, uuid.New())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := r.Cleanup(ctx, ws); err != nil { // simulate the reaper
		t.Fatalf("Cleanup: %v", err)
	}

	if err := r.Ensure(ctx, ws); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if fi, err := os.Stat(ws); err != nil || !fi.IsDir() {
		t.Fatalf("workspace not recreated: %v", err)
	}
}

func TestEnsureIsIdempotentOnAnExistingWorkspace(t *testing.T) {
	base := t.TempDir()
	r := &WorktreeRunner{BaseDir: base}
	ctx := context.Background()

	ws, err := r.Prepare(ctx, uuid.New())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	marker := filepath.Join(ws, "keep-me")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := r.Ensure(ctx, ws); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("Ensure clobbered existing workspace contents: %v", err)
	}
}
