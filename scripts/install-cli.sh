#!/usr/bin/env bash
set -euo pipefail

REPO="${NOVA_REPO:-luaxlou/nova-run}"
VERSION="${NOVA_VERSION:-latest}"
INSTALL_DIR="${NOVA_INSTALL_DIR:-}"
FORCE=0

for arg in "$@"; do
  case "$arg" in
    -f | --force) FORCE=1 ;;
    *)
      echo "Unknown argument: $arg" >&2
      echo "Usage: install-cli.sh [-f|--force]" >&2
      exit 1
      ;;
  esac
done

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

TARGET="${INSTALL_DIR}/nova"
RELEASE_TAG="$VERSION"
TARGET_VERSION="${VERSION#v}"

if [ "$FORCE" -eq 0 ] && [ "$VERSION" = "latest" ]; then
  RELEASE_JSON="$(curl --fail --location --show-error --connect-timeout 20 --max-time 60 --retry 3 --retry-delay 2 --retry-all-errors --silent "https://api.github.com/repos/${REPO}/releases/latest")"
  RELEASE_TAG="$(printf '%s\n' "$RELEASE_JSON" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [ -z "$RELEASE_TAG" ]; then
    echo "Unable to determine the latest Nova version." >&2
    exit 1
  fi
  TARGET_VERSION="${RELEASE_TAG#v}"
fi

if [ "$FORCE" -eq 0 ] && [ -x "$TARGET" ]; then
  LOCAL_OUTPUT="$("$TARGET" version </dev/null 2>/dev/null || true)"
  LOCAL_VERSION="$(printf '%s\n' "$LOCAL_OUTPUT" | awk 'NR == 1 { print $NF }')"
  LOCAL_VERSION="${LOCAL_VERSION#v}"
  if [ -n "$LOCAL_VERSION" ] && [ "$LOCAL_VERSION" = "$TARGET_VERSION" ]; then
    echo "nova ${LOCAL_VERSION} is already the latest version at ${TARGET}; no update needed."
    echo "Run again with --force to reinstall it."
    exit 0
  fi
fi

if [ "$VERSION" = "latest" ] && [ "$FORCE" -eq 1 ]; then
  URL="https://github.com/${REPO}/releases/latest/download/nova-${OS}-${ARCH}"
else
  URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/nova-${OS}-${ARCH}"
fi

echo "Installing nova client"
echo "Platform: ${OS}-${ARCH}"
echo "Download: ${URL}"
echo "Target: ${INSTALL_DIR}/nova"

TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT

curl --fail --location --show-error --connect-timeout 20 --max-time 600 --retry 3 --retry-delay 2 --retry-all-errors --progress-bar "$URL" -o "$TMP_FILE"
chmod +x "$TMP_FILE"

mkdir -p "$INSTALL_DIR"
if [ ! -w "$INSTALL_DIR" ]; then
  echo "${INSTALL_DIR} is not writable."
  echo "Run with NOVA_INSTALL_DIR set to a writable directory, for example:"
  echo "  NOVA_INSTALL_DIR=\"${HOME}/.local/bin\" curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install-cli.sh | bash"
  exit 1
fi

if [ "$FORCE" -eq 0 ] && [ -x "$TARGET" ] && cmp -s "$TMP_FILE" "$TARGET"; then
  echo "nova is already the latest version at ${TARGET}; no update needed."
  echo "Run again with --force to reinstall it."
  exit 0
fi

mv "$TMP_FILE" "$TARGET"
echo "nova installed to ${TARGET}"

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    echo "${INSTALL_DIR} is not in PATH."
    echo "Add it with:"
    echo "  export PATH=\"${INSTALL_DIR}:$PATH\""
    ;;
esac
