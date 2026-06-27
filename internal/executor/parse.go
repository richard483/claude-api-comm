package executor

import (
	"bytes"
	"encoding/json"

	"github.com/richard483/claude-api-comm/internal/model"
)

type streamLine struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Session string `json:"session_id"`
	Result  string `json:"result"`
	NumTurns int   `json:"num_turns"`
	Cost    float64 `json:"total_cost_usd"`
	Duration int64  `json:"duration_ms"`
	Message struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
			Name string `json:"name"`
		} `json:"content"`
	} `json:"message"`
}

// parseLine parses one NDJSON stream-json line from `claude --output-format stream-json`.
func parseLine(line []byte) (model.Event, string, *model.Result, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return model.Event{}, "", nil, false
	}
	var sl streamLine
	if err := json.Unmarshal(line, &sl); err != nil {
		return model.Event{}, "", nil, false
	}
	raw := json.RawMessage(append([]byte(nil), line...))

	switch sl.Type {
	case "system":
		return model.Event{Type: "status", Text: sl.Subtype, Raw: raw}, sl.Session, nil, true
	case "assistant":
		for _, c := range sl.Message.Content {
			switch c.Type {
			case "text":
				return model.Event{Type: "text", Text: c.Text, Raw: raw}, "", nil, true
			case "tool_use":
				return model.Event{Type: "tool_use", Text: c.Name, Raw: raw}, "", nil, true
			}
		}
		return model.Event{Type: "status", Raw: raw}, "", nil, true
	case "result":
		final := &model.Result{
			ClaudeSessionID: sl.Session,
			Text:            sl.Result,
			NumTurns:        sl.NumTurns,
			CostUSD:         sl.Cost,
			DurationMs:      sl.Duration,
		}
		return model.Event{Type: "result", Text: sl.Result, Raw: raw}, sl.Session, final, true
	default:
		return model.Event{Type: "status", Text: sl.Type, Raw: raw}, sl.Session, nil, true
	}
}
