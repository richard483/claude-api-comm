#!/usr/bin/env bash
# uninstall.sh — stop and remove the claude-api-comm systemd service and binary.
#
# Usage:
#   ./deploy/uninstall.sh            stop, disable, and remove the service (needs root; self-sudos)
#   ./deploy/uninstall.sh --dry-run  print what would be removed, then exit
set -euo pipefail

: "${SERVICE_NAME:=claude-api-comm}"
: "${INSTALL_BIN:=/usr/local/bin/claude-api-comm}"

UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

if [ "$DRY_RUN" -eq 1 ]; then
  echo "# would run:    systemctl disable --now ${SERVICE_NAME}"
  echo "# would remove: ${UNIT_PATH}"
  echo "# would remove: ${INSTALL_BIN}"
  echo "# would run:    systemctl daemon-reload"
  exit 0
fi

if [ "$(id -u)" -ne 0 ]; then
  echo "Not root; re-executing with sudo..." >&2
  exec sudo -E "$0" "$@"
fi

echo "Stopping and disabling ${SERVICE_NAME} ..."
systemctl disable --now "$SERVICE_NAME" 2>/dev/null || echo "  (service not active/installed)"

echo "Removing ${UNIT_PATH} and ${INSTALL_BIN} ..."
rm -f "$UNIT_PATH" "$INSTALL_BIN"

systemctl daemon-reload
echo "Done. ${SERVICE_NAME} removed."
