#!/bin/bash
# Install or replace the Go binary under /opt/<APP_NAME>/bin/new-api.
# Run as root. Adjust APP_NAME if you changed it in 01-setup-server.sh.
#
# The binary must be a Linux ELF for this host's CPU (usually linux/amd64).
# If you built on macOS or Windows without cross-compile, you will get
# "cannot execute binary file: Exec format error" — build with:
#   ./scripts/build/build-linux.sh
set -euo pipefail

APP_NAME='koooyooo-newapi'
APP_USER="${APP_NAME}"
BASE="/opt/${APP_NAME}"
DEST="${BASE}/bin/new-api"
SRC="${1:?usage: $0 /path/to/new-api-binary}"

if [[ ! -f "${SRC}" ]]; then
  echo "error: source binary not found: ${SRC}" >&2
  exit 1
fi

install -d -o "${APP_USER}" -g "${APP_USER}" -m 0750 "${BASE}/bin"
install -m 0755 -o "${APP_USER}" -g "${APP_USER}" "${SRC}" "${DEST}"

echo "Installed ${DEST}"
echo "Reload/restart your process manager (systemd, supervisord, etc.) to pick up changes."
echo "Example: systemctl restart ${APP_NAME}.service"
