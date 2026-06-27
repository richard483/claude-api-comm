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

## Deploy as a systemd service

`deploy/install.sh` builds the binary, bakes the config into a systemd unit, and starts it on
boot (restart on crash, logs to journald). Config values are overridable from the environment;
edit the defaults at the top of `deploy/install.sh` or pass them inline.

    # preview the unit that would be written (no root, touches nothing):
    DATABASE_URL="postgres://user:pass@222.222.1.103:5432/agent_memory" \
      ./deploy/install.sh --dry-run

    # install + enable + start (self-sudos if not root):
    DATABASE_URL="postgres://user:pass@222.222.1.103:5432/agent_memory" \
      ./deploy/install.sh

Day-to-day control:

    systemctl start|stop|restart|status claude-api-comm
    journalctl -u claude-api-comm -f

Remove it:

    ./deploy/uninstall.sh            # stop, disable, remove unit + binary
    ./deploy/uninstall.sh --dry-run  # preview only

Notes: Linux/systemd only. The installer creates `WORKSPACE_BASE_DIR` owned by `RUN_USER`
(default `nephren`) and installs the binary to `/usr/local/bin/claude-api-comm`.
