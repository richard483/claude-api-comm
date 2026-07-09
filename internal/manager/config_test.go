package manager

import (
	"testing"

	"github.com/richard483/claude-api-comm/internal/settings"
)

// The config methods only touch settings, not the store, so they run without a
// database. A nil store is safe here for that reason.
func newConfigMgr(seed string) *Manager {
	return New(nil, nil, nil, nil, settings.New(seed), 1, 0)
}

func TestManagerGetConfigReflectsSeed(t *testing.T) {
	m := newConfigMgr("claude-opus-4-8")
	if got := m.GetConfig().Model; got != "claude-opus-4-8" {
		t.Errorf("GetConfig().Model = %q, want claude-opus-4-8", got)
	}
}

func TestManagerSetModelValidAndInvalid(t *testing.T) {
	m := newConfigMgr("")
	cfg, err := m.SetModel("claude-sonnet-5")
	if err != nil {
		t.Fatalf("SetModel valid error = %v", err)
	}
	if cfg.Model != "claude-sonnet-5" || m.GetConfig().Model != "claude-sonnet-5" {
		t.Errorf("model not applied: cfg=%q current=%q", cfg.Model, m.GetConfig().Model)
	}
	if _, err := m.SetModel("gpt-5"); err == nil {
		t.Fatal("expected error for unknown model")
	}
	if got := m.GetConfig().Model; got != "claude-sonnet-5" {
		t.Errorf("model changed after rejected set: %q", got)
	}
}

func TestManagerListModelsNonEmpty(t *testing.T) {
	if len(newConfigMgr("").ListModels()) == 0 {
		t.Fatal("ListModels returned no models")
	}
}
