package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/richard483/claude-api-comm/internal/model"
)

type Store struct{ pool *pgxpool.Pool }

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

const schemaDDL = `
CREATE SCHEMA IF NOT EXISTS comm_agent_memory;
CREATE TABLE IF NOT EXISTS comm_agent_memory.sessions (
	id                UUID PRIMARY KEY,
	claude_session_id TEXT,
	workspace_path    TEXT NOT NULL,
	backend           TEXT NOT NULL,
	label             TEXT NOT NULL DEFAULT '',
	owner             TEXT NOT NULL DEFAULT '',
	status            TEXT NOT NULL,
	rolling_summary   TEXT,
	created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
	last_used_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS comm_agent_memory.turns (
	id          UUID PRIMARY KEY,
	session_id  UUID NOT NULL REFERENCES comm_agent_memory.sessions(id),
	prompt      TEXT NOT NULL,
	status      TEXT NOT NULL,
	result      TEXT NOT NULL DEFAULT '',
	error       TEXT NOT NULL DEFAULT '',
	num_turns   INT NOT NULL DEFAULT 0,
	cost_usd    DOUBLE PRECISION NOT NULL DEFAULT 0,
	duration_ms BIGINT NOT NULL DEFAULT 0,
	created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
	finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS turns_session_idx ON comm_agent_memory.turns(session_id, created_at);
`

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaDDL)
	return err
}

func (s *Store) CreateSession(ctx context.Context, m model.Session) (model.Session, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO comm_agent_memory.sessions
			(id, claude_session_id, workspace_path, backend, label, owner, status, rolling_summary)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING created_at, last_used_at`,
		m.ID, m.ClaudeSessionID, m.WorkspacePath, m.Backend, m.Label, m.Owner, m.Status, m.RollingSummary)
	if err := row.Scan(&m.CreatedAt, &m.LastUsedAt); err != nil {
		return model.Session{}, err
	}
	return m, nil
}

func scanSession(row interface {
	Scan(dest ...any) error
}) (model.Session, error) {
	var m model.Session
	err := row.Scan(&m.ID, &m.ClaudeSessionID, &m.WorkspacePath, &m.Backend,
		&m.Label, &m.Owner, &m.Status, &m.RollingSummary, &m.CreatedAt, &m.LastUsedAt)
	return m, err
}

const sessionCols = `id, claude_session_id, workspace_path, backend, label, owner, status, rolling_summary, created_at, last_used_at`

func (s *Store) GetSession(ctx context.Context, id uuid.UUID) (model.Session, error) {
	return scanSession(s.pool.QueryRow(ctx,
		`SELECT `+sessionCols+` FROM comm_agent_memory.sessions WHERE id=$1`, id))
}

func (s *Store) ListSessions(ctx context.Context, status string) ([]model.Session, error) {
	q := `SELECT ` + sessionCols + ` FROM comm_agent_memory.sessions`
	args := []any{}
	if status != "" {
		q += ` WHERE status=$1`
		args = append(args, status)
	} else {
		q += ` WHERE status <> 'archived'`
	}
	q += ` ORDER BY last_used_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Session
	for rows.Next() {
		m, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSessionClaudeID(ctx context.Context, id uuid.UUID, claudeID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE comm_agent_memory.sessions SET claude_session_id=$2 WHERE id=$1`, id, claudeID)
	return err
}

func (s *Store) TouchSession(ctx context.Context, id uuid.UUID, status model.SessionStatus) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE comm_agent_memory.sessions SET status=$2, last_used_at=now() WHERE id=$1`, id, status)
	return err
}

func (s *Store) SetRollingSummary(ctx context.Context, id uuid.UUID, summary string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE comm_agent_memory.sessions SET rolling_summary=$2 WHERE id=$1`, id, summary)
	return err
}

func (s *Store) CreateTurn(ctx context.Context, m model.Turn) (model.Turn, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO comm_agent_memory.turns (id, session_id, prompt, status)
		VALUES ($1,$2,$3,$4) RETURNING created_at`,
		m.ID, m.SessionID, m.Prompt, m.Status)
	if err := row.Scan(&m.CreatedAt); err != nil {
		return model.Turn{}, err
	}
	return m, nil
}

const turnCols = `id, session_id, prompt, status, result, error, num_turns, cost_usd, duration_ms, created_at, finished_at`

func (s *Store) GetTurn(ctx context.Context, id uuid.UUID) (model.Turn, error) {
	var m model.Turn
	err := s.pool.QueryRow(ctx,
		`SELECT `+turnCols+` FROM comm_agent_memory.turns WHERE id=$1`, id).
		Scan(&m.ID, &m.SessionID, &m.Prompt, &m.Status, &m.Result, &m.Error,
			&m.NumTurns, &m.CostUSD, &m.DurationMs, &m.CreatedAt, &m.FinishedAt)
	return m, err
}

func (s *Store) StartTurn(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE comm_agent_memory.turns SET status='running' WHERE id=$1`, id)
	return err
}

func (s *Store) FinishTurn(ctx context.Context, m model.Turn) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE comm_agent_memory.turns
		SET status=$2, result=$3, error=$4, num_turns=$5, cost_usd=$6, duration_ms=$7, finished_at=now()
		WHERE id=$1`,
		m.ID, m.Status, m.Result, m.Error, m.NumTurns, m.CostUSD, m.DurationMs)
	return err
}
