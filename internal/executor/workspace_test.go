package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/richard483/claude-api-comm/internal/model"
)

// A missing workspace must be reported as a missing workspace. Because Run sets
// cmd.SysProcAttr, Go surfaces the child's failed chdir(cmd.Dir) as
// "fork/exec <Bin>: no such file or directory" — naming the binary, not the
// directory. That message sent us chasing a perfectly healthy claude binary.
func TestRunReportsMissingWorkspaceRatherThanBlamingTheBinary(t *testing.T) {
	bin, err := filepath.Abs("testdata/fake-claude.sh")
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "reaped-workspace")

	ex := &ClaudeExecutor{Bin: bin}
	_, err = ex.Run(context.Background(), missing, "hi", "", func(model.Event) {})
	if err == nil {
		t.Fatal("Run returned nil error for a missing workspace")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error should name the missing workspace %q, got: %v", missing, err)
	}
	if strings.Contains(err.Error(), "fork/exec") {
		t.Errorf("error blames the binary instead of the workspace: %v", err)
	}
}

// A workspace path that exists but is a regular file is equally unusable as a cwd.
func TestRunRejectsWorkspaceThatIsNotADirectory(t *testing.T) {
	bin, err := filepath.Abs("testdata/fake-claude.sh")
	if err != nil {
		t.Fatal(err)
	}
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ex := &ClaudeExecutor{Bin: bin}
	_, err = ex.Run(context.Background(), notADir, "hi", "", func(model.Event) {})
	if err == nil {
		t.Fatal("Run returned nil error for a workspace that is not a directory")
	}
	if !strings.Contains(err.Error(), notADir) {
		t.Errorf("error should name %q, got: %v", notADir, err)
	}
}
