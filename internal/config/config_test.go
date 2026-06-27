package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	// all optional vars unset -> defaults apply
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ListenAddr != ":18100" {
		t.Errorf("ListenAddr = %q, want :18100", cfg.ListenAddr)
	}
	if cfg.ClaudeBin != "claude" {
		t.Errorf("ClaudeBin = %q, want claude", cfg.ClaudeBin)
	}
	if cfg.MaxConcurrency != 3 {
		t.Errorf("MaxConcurrency = %d, want 3", cfg.MaxConcurrency)
	}
	if cfg.TurnTimeout != 30*time.Minute {
		t.Errorf("TurnTimeout = %v, want 30m", cfg.TurnTimeout)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("MAX_CONCURRENCY", "8")
	t.Setenv("TURN_TIMEOUT", "5m")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConcurrency != 8 {
		t.Errorf("MaxConcurrency = %d, want 8", cfg.MaxConcurrency)
	}
	if cfg.TurnTimeout != 5*time.Minute {
		t.Errorf("TurnTimeout = %v, want 5m", cfg.TurnTimeout)
	}
}
