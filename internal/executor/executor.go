package executor

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"

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
	args := []string{"-p", prompt, "--output-format", "stream-json", "--dangerously-skip-permissions"}
	if e.Model != "" {
		args = append(args, "--model", e.Model)
	}
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
	}
	cmd := exec.CommandContext(ctx, e.Bin, args...)
	cmd.Dir = workspace

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
	if err := cmd.Wait(); err != nil {
		return model.Result{}, fmt.Errorf("claude exited: %w", err)
	}
	if final == nil {
		return model.Result{}, fmt.Errorf("no result event from claude")
	}
	return *final, nil
}
