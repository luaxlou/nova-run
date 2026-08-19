#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
APP_SOURCE="${APP_SOURCE:-${APP:-nova}}"
APP_NAME="${APP_NAME:-nova}"
TOKEN_FILE="${TOKEN_FILE:-/etc/nova/token}"
LISTEN_ADDR="${LISTEN_ADDR:-:32102}"
APP_ROOT="${APP_ROOT:-/var/lib/nova/apps}"
SERVICE_NAME="${SERVICE_NAME:-nova.service}"
APP_BIN="${APP_SOURCE}"

install -m 0755 "${APP_BIN}" "${INSTALL_DIR}/${APP_NAME}"
mkdir -p "${APP_ROOT}"

cat > /tmp/nova-service.tpl <<EOF
[Unit]
Description=Nova Runtime
After=network.target

[Service]
Type=simple
WorkingDirectory=__APP_ROOT__
ExecStart=__INSTALL_DIR__/__APP_NAME__ agent -listen __LISTEN__ -app-root __APP_ROOT__ -token-file __TOKEN__
Restart=on-failure
RestartSec=2
KillSignal=SIGTERM
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
EOF

sed -i "s|__LISTEN__|${LISTEN_ADDR}|" /tmp/nova-service.tpl
sed -i "s|__APP_ROOT__|${APP_ROOT}|" /tmp/nova-service.tpl
sed -i "s|__TOKEN__|${TOKEN_FILE}|" /tmp/nova-service.tpl
sed -i "s|__INSTALL_DIR__|${INSTALL_DIR}|" /tmp/nova-service.tpl
sed -i "s|__APP_NAME__|${APP_NAME}|" /tmp/nova-service.tpl

mv /tmp/nova-service.tpl /etc/systemd/system/${SERVICE_NAME}

mv /tmp/${SERVICE_NAME} /etc/systemd/system/${SERVICE_NAME}
systemctl daemon-reload
systemctl enable --now ${SERVICE_NAME}

echo "nova installed, binary: ${INSTALL_DIR}/${APP}, unit: ${SERVICE_NAME}"
echo "token file: ${TOKEN_FILE}"
