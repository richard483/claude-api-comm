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

## Interactive docs

Swagger UI with "Try it out" is served at `http://<host>:18100/docs` (raw spec at
`/openapi.yaml`). The UI loads its assets from a CDN, so opening `/docs` needs browser internet.

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

## Run with Docker

The image bundles the Go binary plus Node and the `claude` CLI (and `git`, for session
worktrees). It carries **no Claude credentials** — you must mount them at run time.

    docker build -t claude-api-comm .

    docker run -d --name claude-api-comm \
      -p 18100:18100 \
      -e DATABASE_URL="postgres://user:pass@222.222.1.103:5432/agent_memory" \
      -e WORKSPACE_BASE_DIR=/sessions \
      -v "$HOME/.claude:/root/.claude" \                 # REQUIRED: Claude auth (read-write, for token refresh)
      -v /home/nephren/claude-sessions:/sessions \       # persist session worktrees on the host
      --restart unless-stopped \
      claude-api-comm

The `/sessions` mount above is a **bind mount** to a host path, so the worktrees live at
`/home/nephren/claude-sessions` and are easy to inspect on the VM. Docker auto-creates that path
if missing (root-owned, which is fine since the container runs as root). If you'd rather let Docker
manage the storage instead, replace it with a **named volume** — `-v claude-sessions:/sessions` —
which Docker creates automatically (no host directory needed).

Required at run time:
- **`-v "$HOME/.claude:/root/.claude"`** — without your Claude credentials mounted, the `claude`
  CLI inside the container cannot authenticate. Mount read-write so refreshed tokens persist.
  The container runs as root with `HOME=/root`, so the mount target is `/root/.claude`. If you run
  with a non-root `--user`, mount to that user's home and ensure it can read the creds.
- **`-e DATABASE_URL=...`** — required; the container must be able to reach Postgres at
  `222.222.1.103:5432`.

Optional env (same as the binary): `LISTEN_ADDR`, `DEFAULT_MODEL`, `CLAUDE_BIN`,
`MAX_CONCURRENCY`, `TURN_TIMEOUT`, `IDLE_REAP_AGE`.

Note: in Docker, `claude` runs *inside the container*, so its full-access execution is bounded by
the container. All sessions still share one container — this containerizes the service, not the
per-session container isolation backend (a separate future item).
