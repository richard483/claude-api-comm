#!/usr/bin/env bash
# install.sh — build claude-api-comm and install it as a systemd service.
#
# Config values below are overridable from the environment, e.g.:
#   DATABASE_URL="postgres://..." RUN_USER=svc ./deploy/install.sh
#
# Usage:
#   ./deploy/install.sh            install/update the service and start it (needs root; self-sudos)
#   ./deploy/install.sh --dry-run  print the systemd unit that would be written, then exit
set -euo pipefail

# ---- config (override via environment) ----
: "${SERVICE_NAME:=claude-api-comm}"
: "${INSTALL_BIN:=/usr/local/bin/claude-api-comm}"
: "${RUN_USER:=nephren}"

# app env baked into the unit as Environment= lines.
# NOTE: do not commit real credentials here — pass a DSN with a password via the environment,
#   e.g.  DATABASE_URL="postgres://user:pass@host:5432/db" ./deploy/install.sh
: "${DATABASE_URL:=postgres://agent_memory@222.222.1.103:5432/agent_memory}"
: "${LISTEN_ADDR:=:18100}"
: "${WORKSPACE_BASE_DIR:=/home/nephren/claude-sessions}"
# Absolute path to the claude CLI. systemd uses a minimal PATH and does NOT load the user's
# shell profile, so a bare "claude" (installed under ~/.local/bin or nvm) won't be found.
# Resolve it at install time; fall back to the native-install default.
: "${CLAUDE_BIN:=$(command -v claude 2>/dev/null || echo "/home/${RUN_USER}/.local/bin/claude")}"
: "${DEFAULT_MODEL:=}"
: "${MAX_CONCURRENCY:=3}"
: "${TURN_TIMEOUT:=30m}"
: "${IDLE_REAP_AGE:=24h}"

UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

# Render the unit to stdout. Environment values are quoted so spaces/colons survive.
render_unit() {
  cat <<EOF
[Unit]
Description=claude-api-comm — headless Claude HTTP API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${RUN_USER}
WorkingDirectory=${WORKSPACE_BASE_DIR}
Environment="PATH=$(dirname "${CLAUDE_BIN}"):/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
Environment="DATABASE_URL=${DATABASE_URL}"
Environment="LISTEN_ADDR=${LISTEN_ADDR}"
Environment="WORKSPACE_BASE_DIR=${WORKSPACE_BASE_DIR}"
Environment="CLAUDE_BIN=${CLAUDE_BIN}"
Environment="DEFAULT_MODEL=${DEFAULT_MODEL}"
Environment="MAX_CONCURRENCY=${MAX_CONCURRENCY}"
Environment="TURN_TIMEOUT=${TURN_TIMEOUT}"
Environment="IDLE_REAP_AGE=${IDLE_REAP_AGE}"
ExecStart=${INSTALL_BIN}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
}

if [ "$DRY_RUN" -eq 1 ]; then
  echo "# would build:   go build -o ${INSTALL_BIN} ./cmd/server  (cwd ${REPO_ROOT})"
  echo "# would write:   ${UNIT_PATH}"
  echo "# would run:     systemctl daemon-reload && systemctl enable --now ${SERVICE_NAME}"
  echo "# ---- ${UNIT_PATH} ----"
  render_unit
  exit 0
fi

# Re-exec under sudo if not root (preserving the overridable config).
if [ "$(id -u)" -ne 0 ]; then
  echo "Not root; re-executing with sudo..." >&2
  exec sudo -E "$0" "$@"
fi

echo "Building ${INSTALL_BIN} ..."
( cd "$REPO_ROOT" && go build -o "$INSTALL_BIN" ./cmd/server )

echo "Ensuring workspace dir ${WORKSPACE_BASE_DIR} (owner ${RUN_USER}) ..."
install -d -o "$RUN_USER" -g "$RUN_USER" "$WORKSPACE_BASE_DIR"

echo "Writing ${UNIT_PATH} ..."
render_unit > "$UNIT_PATH"

echo "Reloading systemd and (re)starting ${SERVICE_NAME} ..."
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
# restart (not just start) so a re-install applies the updated unit/env to a running service.
systemctl restart "$SERVICE_NAME"

echo
systemctl --no-pager --full status "$SERVICE_NAME" || true
echo
echo "Done. Manage with: systemctl {start,stop,restart,status} ${SERVICE_NAME}"
echo "Logs: journalctl -u ${SERVICE_NAME} -f"
