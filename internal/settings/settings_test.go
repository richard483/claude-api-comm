package settings

import "testing"

func TestModelDefaultsToSeed(t *testing.T) {
	s := New("claude-opus-4-8")
	if got := s.Model(); got != "claude-opus-4-8" {
		t.Errorf("Model() = %q, want claude-opus-4-8", got)
	}
}

func TestSetModelKnownName(t *testing.T) {
	s := New("")
	if err := s.SetModel("claude-sonnet-5"); err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if got := s.Model(); got != "claude-sonnet-5" {
		t.Errorf("Model() = %q, want claude-sonnet-5", got)
	}
}

func TestSetModelAlias(t *testing.T) {
	s := New("")
	if err := s.SetModel("opus"); err != nil {
		t.Fatalf("SetModel(alias) error = %v", err)
	}
	if got := s.Model(); got != "opus" {
		t.Errorf("Model() = %q, want opus", got)
	}
}

func TestSetModelEmptyResets(t *testing.T) {
	s := New("claude-opus-4-8")
	if err := s.SetModel(""); err != nil {
		t.Fatalf("SetModel(\"\") error = %v", err)
	}
	if got := s.Model(); got != "" {
		t.Errorf("Model() = %q, want empty", got)
	}
}

func TestSetModelUnknownRejected(t *testing.T) {
	s := New("claude-opus-4-8")
	if err := s.SetModel("gpt-5"); err == nil {
		t.Fatal("expected error for unknown model")
	}
	if got := s.Model(); got != "claude-opus-4-8" {
		t.Errorf("Model() = %q, want unchanged claude-opus-4-8", got)
	}
}

func TestIsKnownModel(t *testing.T) {
	if !IsKnownModel("claude-haiku-4-5") {
		t.Error("claude-haiku-4-5 should be known")
	}
	if !IsKnownModel("haiku") {
		t.Error("alias haiku should be known")
	}
	if IsKnownModel("nope") {
		t.Error("nope should not be known")
	}
}
