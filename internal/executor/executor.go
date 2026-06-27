package executor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/richard483/claude-api-comm/internal/model"
)

type Executor interface {
	Run(ctx context.Context, workspace, prompt, resumeID string, emit func(model.Event)) (model.Result, error)
}

type ClaudeExecutor struct {
	Bin   string
	Model string
}

func (e *ClaudeExecutor) Run(ctx context.Context, workspace, prompt, resumeID string, emit func(model.Event)) (model.Result, error) {
	// claude requires --verbose when combining --print (-p) with --output-format stream-json.
	args := []string{"-p", prompt, "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"}
	if e.Model != "" {
		args = append(args, "--model", e.Model)
	}
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
	}
	cmd := exec.CommandContext(ctx, e.Bin, args...)
	cmd.Dir = workspace
	// Fix 3: start child in its own process group so we can kill the whole group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Fix 3: on context cancellation, kill the entire process group.
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second

	// Fix 2: capture stderr so failure detail is preserved.
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return model.Result{}, err
	}
	if err := cmd.Start(); err != nil {
		return model.Result{}, err
	}

	var final *model.Result
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		ev, _, fin, ok := parseLine(scanner.Bytes())
		if !ok {
			continue
		}
		emit(ev)
		if fin != nil {
			final = fin
		}
	}
	// Fix 4: capture scanner error before Wait so it's not lost.
	scanErr := scanner.Err()

	waitErr := cmd.Wait()

	// Fix 2: include stderr in the returned error on non-zero exit.
	if waitErr != nil {
		stderrText := stderrBuf.String()
		const maxStderr = 4096
		if len(stderrText) > maxStderr {
			stderrText = stderrText[len(stderrText)-maxStderr:]
		}
		return model.Result{}, fmt.Errorf("claude exited: %w: %s", waitErr, stderrText)
	}
	// Fix 4: surface scanner error if process succeeded but we had a read failure.
	if scanErr != nil {
		return model.Result{}, fmt.Errorf("reading claude output: %w", scanErr)
	}
	if final == nil {
		return model.Result{}, fmt.Errorf("no result event from claude")
	}
	return *final, nil
}
