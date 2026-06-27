package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type SessionStatus string

const (
	SessionActive   SessionStatus = "active"
	SessionIdle     SessionStatus = "idle"
	SessionArchived SessionStatus = "archived"
)

type TurnStatus string

const (
	TurnQueued  TurnStatus = "queued"
	TurnRunning TurnStatus = "running"
	TurnDone    TurnStatus = "done"
	TurnError   TurnStatus = "error"
)

func (s TurnStatus) Terminal() bool {
	return s == TurnDone || s == TurnError
}

type Session struct {
	ID              uuid.UUID     `json:"id"`
	ClaudeSessionID *string       `json:"claude_session_id"`
	WorkspacePath   string        `json:"workspace_path"`
	Backend         string        `json:"backend"`
	Label           string        `json:"label"`
	Owner           string        `json:"owner"`
	Status          SessionStatus `json:"status"`
	RollingSummary  *string       `json:"rolling_summary"`
	CreatedAt       time.Time     `json:"created_at"`
	LastUsedAt      time.Time     `json:"last_used_at"`
}

type Turn struct {
	ID         uuid.UUID  `json:"id"`
	SessionID  uuid.UUID  `json:"session_id"`
	Prompt     string     `json:"prompt"`
	Status     TurnStatus `json:"status"`
	Result     string     `json:"result"`
	Error      string     `json:"error"`
	NumTurns   int        `json:"num_turns"`
	CostUSD    float64    `json:"cost_usd"`
	DurationMs int64      `json:"duration_ms"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type Event struct {
	Type string          `json:"type"`
	Text string          `json:"text,omitempty"`
	Raw  json.RawMessage `json:"raw,omitempty"`
}

type Result struct {
	ClaudeSessionID string
	Text            string
	NumTurns        int
	CostUSD         float64
	DurationMs      int64
}
