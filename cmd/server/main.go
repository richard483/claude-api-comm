package main

import (
	"context"
	"log"
	"net/http"

	"github.com/richard483/claude-api-comm/internal/api"
	"github.com/richard483/claude-api-comm/internal/broker"
	"github.com/richard483/claude-api-comm/internal/config"
	"github.com/richard483/claude-api-comm/internal/executor"
	"github.com/richard483/claude-api-comm/internal/manager"
	"github.com/richard483/claude-api-comm/internal/runner"
	"github.com/richard483/claude-api-comm/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	run := &runner.WorktreeRunner{BaseDir: cfg.WorkspaceBaseDir}
	ex := &executor.ClaudeExecutor{Bin: cfg.ClaudeBin, Model: cfg.DefaultModel}
	mgr := manager.New(st, run, ex, broker.New(), cfg.MaxConcurrency, cfg.TurnTimeout)

	log.Printf("claude-api-comm listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, api.NewRouter(mgr)); err != nil {
		log.Fatal(err)
	}
}
