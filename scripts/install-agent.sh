#!/usr/bin/env bash
set -euo pipefail

REPO="${NOVA_REPO:-luaxlou/nova-run}"
VERSION="${NOVA_VERSION:-latest}"
INSTALL_DIR="${NOVA_INSTALL_DIR:-/usr/local/bin}"
SERVICE_NAME="${NOVA_SERVICE_NAME:-nova.service}"
TOKEN_FILE="${NOVA_TOKEN_FILE:-/etc/nova/token}"
LISTEN_ADDR="${NOVA_LISTEN_ADDR:-:32102}"
APP_ROOT="${NOVA_APP_ROOT:-/var/lib/nova/apps}"

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run this installer as root, for example:"
  echo "  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install-agent.sh | sudo bash"
  exit 1
fi

case "$(uname -s)" in
  Linux) OS="linux" ;;
  *)
    echo "Unsupported OS: $(uname -s). Nova runtime installation currently supports Linux only."
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) ARCH="amd64" ;;
  arm64 | aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $(uname -m)"
    exit 1
    ;;
esac

if [ "$VERSION" = "latest" ]; then
  URL="https://github.com/${REPO}/releases/latest/download/nova-${OS}-${ARCH}"
else
  URL="https://github.com/${REPO}/releases/download/${VERSION}/nova-${OS}-${ARCH}"
fi

TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT

echo "Downloading nova from ${URL}"
curl -fsSL "$URL" -o "$TMP_FILE"
chmod +x "$TMP_FILE"

mkdir -p "$INSTALL_DIR" "$(dirname "$TOKEN_FILE")" "$APP_ROOT"
mv "$TMP_FILE" "${INSTALL_DIR}/nova"

if [ ! -s "$TOKEN_FILE" ]; then
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32 > "$TOKEN_FILE"
  else
    tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 64 > "$TOKEN_FILE"
    printf '\n' >> "$TOKEN_FILE"
  fi
  chmod 600 "$TOKEN_FILE"
fi

cat > "/etc/systemd/system/${SERVICE_NAME}" <<EOF
[Unit]
Description=Nova Runtime
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/nova agent --listen ${LISTEN_ADDR} --app-root ${APP_ROOT} --token-file ${TOKEN_FILE}
Restart=on-failure
RestartSec=2
KillSignal=SIGTERM
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now "$SERVICE_NAME"

echo "nova runtime installed"
echo "binary: ${INSTALL_DIR}/nova"
echo "service: ${SERVICE_NAME}"
echo "token: ${TOKEN_FILE}"
