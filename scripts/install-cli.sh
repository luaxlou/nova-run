#!/usr/bin/env bash
set -euo pipefail

REPO="${NOVA_REPO:-luaxlou/nova-run}"
VERSION="${NOVA_VERSION:-latest}"
INSTALL_DIR="${NOVA_INSTALL_DIR:-/usr/local/bin}"

case "$(uname -s)" in
  Linux) OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)
    echo "Unsupported OS: $(uname -s)"
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

if [ -w "$INSTALL_DIR" ]; then
  mkdir -p "$INSTALL_DIR"
  mv "$TMP_FILE" "${INSTALL_DIR}/nova"
elif command -v sudo >/dev/null 2>&1; then
  sudo mkdir -p "$INSTALL_DIR"
  sudo mv "$TMP_FILE" "${INSTALL_DIR}/nova"
else
  echo "${INSTALL_DIR} is not writable and sudo is not available."
  echo "Set NOVA_INSTALL_DIR to a writable directory and try again."
  exit 1
fi

echo "nova installed to ${INSTALL_DIR}/nova"
