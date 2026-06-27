package executor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/richard483/claude-api-comm/internal/model"
)

func TestClaudeExecutorRun(t *testing.T) {
	bin, err := filepath.Abs("testdata/fake-claude.sh")
	if err != nil {
		t.Fatal(err)
	}
	ex := &ClaudeExecutor{Bin: bin}
	var events []model.Event
	res, err := ex.Run(context.Background(), t.TempDir(), "hi", "", func(e model.Event) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ClaudeSessionID != "fake-sess" {
		t.Errorf("session id = %q, want fake-sess", res.ClaudeSessionID)
	}
	if res.Text != "all done" || res.NumTurns != 1 {
		t.Errorf("bad result: %+v", res)
	}
	var sawText, sawTool, sawResult bool
	for _, e := range events {
		switch e.Type {
		case "text":
			sawText = true
		case "tool_use":
			sawTool = true
		case "result":
			sawResult = true
		}
	}
	if !sawText || !sawTool || !sawResult {
		t.Errorf("missing events: text=%v tool=%v result=%v", sawText, sawTool, sawResult)
	}
}
