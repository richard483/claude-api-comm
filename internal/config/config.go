package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL      string
	ListenAddr       string
	WorkspaceBaseDir string
	ClaudeBin        string
	DefaultModel     string
	MaxConcurrency   int
	TurnTimeout      time.Duration
	IdleReapAge      time.Duration
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		ListenAddr:       envStr("LISTEN_ADDR", ":18100"),
		WorkspaceBaseDir: envStr("WORKSPACE_BASE_DIR", "/home/nephren/claude-sessions"),
		ClaudeBin:        envStr("CLAUDE_BIN", "claude"),
		DefaultModel:     envStr("DEFAULT_MODEL", ""),
		MaxConcurrency:   envInt("MAX_CONCURRENCY", 3),
		TurnTimeout:      envDur("TURN_TIMEOUT", 30*time.Minute),
		IdleReapAge:      envDur("IDLE_REAP_AGE", 24*time.Hour),
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if c.MaxConcurrency < 1 {
		return Config{}, fmt.Errorf("MAX_CONCURRENCY must be >= 1")
	}
	return c, nil
}

func envStr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
