# claude-api-comm

HTTP API that drives headless Claude Code on a VM: create/resume sessions, send prompts
asynchronously, stream progress over SSE, and persist a session/turn registry in Postgres
(`comm_agent_memory` schema).

## Run

    export DATABASE_URL="postgres://user:pass@222.222.1.103:5432/agent_memory"
    export WORKSPACE_BASE_DIR="/home/nephren/claude-sessions"
    go run ./cmd/server

## Config (env)

| var | default | meaning |
|-----|---------|---------|
| DATABASE_URL | (required) | Postgres DSN |
| LISTEN_ADDR | :18100 | HTTP listen address |
| WORKSPACE_BASE_DIR | /home/nephren/claude-sessions | per-session worktrees |
| CLAUDE_BIN | claude | claude binary path |
| DEFAULT_MODEL | (none) | --model passed to claude |
| MAX_CONCURRENCY | 3 | global concurrent turns |
| TURN_TIMEOUT | 30m | per-turn ceiling |
| IDLE_REAP_AGE | 24h | reserved for idle reaping |

## Endpoints

- `POST /sessions` `{label,owner}` → 201 session
- `GET /sessions?status=` → sessions
- `GET /sessions/{id}` → session
- `POST /sessions/{id}/messages` `{prompt}` → 202 turn (queued)
- `GET /sessions/{id}/turns/{turnID}` → turn status/result (poll)
- `GET /sessions/{id}/turns/{turnID}/stream` → SSE progress
- `POST /sessions/{id}/summarize?fork=true|false` → rolling summary (+ forked session)
- `POST /sessions/{id}/archive` → 204

No auth in v1; deploy on a trusted network behind a verifying caller.
