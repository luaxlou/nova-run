#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="${SERVICE_NAME:-nova.service}"
APP_PATH="${APP_PATH:-/usr/local/bin/nova}"

systemctl stop "${SERVICE_NAME}" || true
systemctl disable "${SERVICE_NAME}" || true
rm -f "/etc/systemd/system/${SERVICE_NAME}"
systemctl daemon-reload || true
rm -f "${APP_PATH}"
echo "nova removed"
