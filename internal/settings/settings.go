// Package settings holds runtime-mutable service configuration — currently the
// model passed to `claude --model` — plus the registry of models callers may
// select. It is safe for concurrent use.
package settings

import (
	"fmt"
	"sync"
)

// Settings is the live, mutable service configuration. The zero value is not
// usable; construct with New.
type Settings struct {
	mu    sync.RWMutex
	model string
}

// New returns Settings seeded with the given model. An empty model means
// "let claude pick its own default" (no --model flag is passed).
func New(model string) *Settings {
	return &Settings{model: model}
}

// Model returns the currently configured model. An empty string means no
// --model flag is passed to claude, so the CLI uses its built-in default.
func (s *Settings) Model() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

// SetModel validates name against KnownModels and, if valid, makes it the
// current model. The empty string is allowed and resets to the claude CLI
// default. An unknown, non-empty name is rejected and the current model is
// left unchanged.
func (s *Settings) SetModel(name string) error {
	if name != "" && !IsKnownModel(name) {
		return fmt.Errorf("unknown model %q", name)
	}
	s.mu.Lock()
	s.model = name
	s.mu.Unlock()
	return nil
}
