package model

import "testing"

func TestTurnStatusTerminal(t *testing.T) {
	cases := map[TurnStatus]bool{
		TurnQueued:  false,
		TurnRunning: false,
		TurnDone:    true,
		TurnError:   true,
	}
	for st, want := range cases {
		if got := st.Terminal(); got != want {
			t.Errorf("%s.Terminal() = %v, want %v", st, got, want)
		}
	}
}
