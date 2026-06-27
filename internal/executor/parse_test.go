package executor

import "testing"

func TestParseInitCapturesSessionID(t *testing.T) {
	line := []byte(`{"type":"system","subtype":"init","session_id":"sess-1","tools":["Bash"]}`)
	ev, sid, final, ok := parseLine(line)
	if !ok {
		t.Fatal("expected ok")
	}
	if sid != "sess-1" {
		t.Errorf("sessionID = %q, want sess-1", sid)
	}
	if final != nil {
		t.Error("init must not be final")
	}
	if ev.Type != "status" {
		t.Errorf("ev.Type = %q, want status", ev.Type)
	}
}

func TestParseAssistantText(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}`)
	ev, _, _, ok := parseLine(line)
	if !ok || ev.Type != "text" || ev.Text != "hello" {
		t.Errorf("got ok=%v ev=%+v", ok, ev)
	}
}

func TestParseToolUse(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`)
	ev, _, _, ok := parseLine(line)
	if !ok || ev.Type != "tool_use" || ev.Text != "Bash" {
		t.Errorf("got ok=%v ev=%+v", ok, ev)
	}
}

func TestParseResult(t *testing.T) {
	line := []byte(`{"type":"result","subtype":"success","result":"done","session_id":"sess-1","num_turns":2,"total_cost_usd":0.05,"duration_ms":4200}`)
	ev, sid, final, ok := parseLine(line)
	if !ok || final == nil {
		t.Fatalf("expected final result, ok=%v final=%v", ok, final)
	}
	if sid != "sess-1" || final.Text != "done" || final.NumTurns != 2 || final.CostUSD != 0.05 || final.DurationMs != 4200 {
		t.Errorf("bad final: %+v sid=%s", final, sid)
	}
	if ev.Type != "result" {
		t.Errorf("ev.Type = %q, want result", ev.Type)
	}
}

func TestParseBlankLine(t *testing.T) {
	if _, _, _, ok := parseLine([]byte("   ")); ok {
		t.Error("blank line should not be ok")
	}
}
