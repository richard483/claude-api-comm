package model

import (
	"encoding/json"
	"testing"
)

// The embedded Turn must flatten into the top-level object, so the body is
// exactly the existing turn representation plus one field.
func TestLatestTurnMarshalsFlatWithPending(t *testing.T) {
	lt := LatestTurn{
		Turn:    Turn{Prompt: "what is 2+2", Result: "4", Status: TurnDone},
		Pending: true,
	}

	b, err := json.Marshal(lt)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["prompt"] != "what is 2+2" {
		t.Errorf("prompt = %v, want it promoted to the top level", got["prompt"])
	}
	if got["result"] != "4" {
		t.Errorf("result = %v", got["result"])
	}
	if got["status"] != "done" {
		t.Errorf("status = %v", got["status"])
	}
	if got["pending"] != true {
		t.Errorf("pending = %v, want true", got["pending"])
	}
	if _, nested := got["Turn"]; nested {
		t.Error(`body has a nested "Turn" key; Turn must be embedded, not a named field`)
	}
}
