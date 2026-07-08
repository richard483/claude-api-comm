package runner

import (
	"context"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type SessionRunner interface {
	Prepare(ctx context.Context, sessionID uuid.UUID) (string, error)
	// Ensure makes an already-recorded workspace usable again, idempotently.
	// Prepare only runs once, at session creation; Cleanup (the idle reaper)
	// can delete a live session's workspace out from under it.
	Ensure(ctx context.Context, workspacePath string) error
	Cleanup(ctx context.Context, workspacePath string) error
	Backend() string
}

type WorktreeRunner struct {
	BaseDir string
}

func (r *WorktreeRunner) Backend() string { return "worktree" }

func (r *WorktreeRunner) Prepare(ctx context.Context, sessionID uuid.UUID) (string, error) {
	ws := filepath.Join(r.BaseDir, sessionID.String())
	if err := r.Ensure(ctx, ws); err != nil {
		return "", err
	}
	return ws, nil
}

func (r *WorktreeRunner) Ensure(_ context.Context, workspacePath string) error {
	return os.MkdirAll(workspacePath, 0o755)
}

func (r *WorktreeRunner) Cleanup(_ context.Context, workspacePath string) error {
	return os.RemoveAll(workspacePath)
}
