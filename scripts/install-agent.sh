#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
APP="${APP:-nova-agent}"
TOKEN_FILE="${TOKEN_FILE:-/etc/nova-agent/token}"
LISTEN_ADDR="${LISTEN_ADDR:-:32102}"
APP_ROOT="${APP_ROOT:-/var/lib/nova/apps}"

install -m 0755 "${APP}" "${INSTALL_DIR}/${APP}"
mkdir -p "${APP_ROOT}"

cat > /tmp/nova-agent.service <<'EOF'
[Unit]
Description=Nova Run Agent
After=network.target

[Service]
Type=simple
WorkingDirectory=__APP_ROOT__
ExecStart=__INSTALL_DIR__/nova-agent -listen __LISTEN__ -app-root __APP_ROOT__ -token-file __TOKEN__
Restart=on-failure
RestartSec=2
KillSignal=SIGTERM
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
EOF

sed -i "s|__LISTEN__|${LISTEN_ADDR}|" /tmp/nova-agent.service
sed -i "s|__APP_ROOT__|${APP_ROOT}|" /tmp/nova-agent.service
sed -i "s|__TOKEN__|${TOKEN_FILE}|" /tmp/nova-agent.service
sed -i "s|__INSTALL_DIR__|${INSTALL_DIR}|" /tmp/nova-agent.service

mv /tmp/nova-agent.service /etc/systemd/system/nova-agent.service
systemctl daemon-reload
systemctl enable --now nova-agent.service

echo "nova-agent installed, binary: ${INSTALL_DIR}/${APP}, unit: nova-agent.service"
echo "token file: ${TOKEN_FILE}"
