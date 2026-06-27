package runner

import (
	"context"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type SessionRunner interface {
	Prepare(ctx context.Context, sessionID uuid.UUID) (string, error)
	Cleanup(ctx context.Context, workspacePath string) error
	Backend() string
}

type WorktreeRunner struct {
	BaseDir string
}

func (r *WorktreeRunner) Backend() string { return "worktree" }

func (r *WorktreeRunner) Prepare(_ context.Context, sessionID uuid.UUID) (string, error) {
	ws := filepath.Join(r.BaseDir, sessionID.String())
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return "", err
	}
	return ws, nil
}

func (r *WorktreeRunner) Cleanup(_ context.Context, workspacePath string) error {
	return os.RemoveAll(workspacePath)
}
