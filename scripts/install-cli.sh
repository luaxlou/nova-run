#!/usr/bin/env bash
set -euo pipefail

REPO="${NOVA_REPO:-luaxlou/nova-run}"
VERSION="${NOVA_VERSION:-latest}"
INSTALL_DIR="${NOVA_INSTALL_DIR:-}"

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

if [ -z "$INSTALL_DIR" ]; then
  EXISTING_NOVA="$(command -v nova || true)"
  if [ -n "$EXISTING_NOVA" ] && [ -w "$(dirname "$EXISTING_NOVA")" ]; then
    INSTALL_DIR="$(dirname "$EXISTING_NOVA")"
  elif [ -w /usr/local/bin ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${HOME}/.local/bin"
  fi
fi

TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT

echo "Installing nova client"
echo "Platform: ${OS}-${ARCH}"
echo "Download: ${URL}"
echo "Target: ${INSTALL_DIR}/nova"

curl --fail --location --show-error --connect-timeout 10 --max-time 300 --retry 2 --progress-bar "$URL" -o "$TMP_FILE"
chmod +x "$TMP_FILE"

mkdir -p "$INSTALL_DIR"
if [ ! -w "$INSTALL_DIR" ]; then
  echo "${INSTALL_DIR} is not writable."
  echo "Run with NOVA_INSTALL_DIR set to a writable directory, for example:"
  echo "  NOVA_INSTALL_DIR=\"${HOME}/.local/bin\" curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install-cli.sh | bash"
  exit 1
fi

mv "$TMP_FILE" "${INSTALL_DIR}/nova"
echo "nova installed to ${INSTALL_DIR}/nova"

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    echo "${INSTALL_DIR} is not in PATH."
    echo "Add it with:"
    echo "  export PATH=\"${INSTALL_DIR}:$PATH\""
    ;;
esac
