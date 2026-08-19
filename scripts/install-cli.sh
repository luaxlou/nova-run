#!/usr/bin/env bash
set -euo pipefail

repo="${NOVA_GITHUB_REPO:-}"
if [ -z "$repo" ]; then
  repo="$(detect_default_repo)"
fi
if [ -z "$repo" ]; then
  echo "未设置 NOVA_GITHUB_REPO，已回退到默认仓库 luaxlou/nova-run。可设置 NOVA_GITHUB_REPO 覆盖。"
  exit 1
fi

binary_name="${NOVA_BINARY_NAME:-nova}"
install_dir="${NOVA_INSTALL_DIR:-}"
manual_url="${NOVA_CLIENT_DOWNLOAD_URL:-}"

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux*) os="linux" ;;
    darwin*) os="darwin" ;;
    mingw*|msys*|cygwin*) os="windows" ;;
    *) os="" ;;
  esac

  arch="$(uname -m | tr '[:upper:]' '[:lower:]')"
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    armv7*|armv6*) arch="arm" ;;
    i386|i686) arch="386" ;;
    *) arch="" ;;
  esac

  if [ -z "$os" ] || [ -z "$arch" ]; then
    echo "不支持当前运行平台：$(uname -s)/$(uname -m)"
    exit 1
  fi

  PLATFORM_OS="$os"
  PLATFORM_ARCH="$arch"
}

choose_install_dir() {
  if [ -n "$install_dir" ]; then
    NOVA_BIN_DIR="$install_dir"
    SUDO_PREFIX=""
    return
  fi

  if [ -w /usr/local/bin ]; then
    NOVA_BIN_DIR="/usr/local/bin"
    SUDO_PREFIX=""
  elif command -v sudo >/dev/null 2>&1 && [ -d /usr/local/bin ]; then
    NOVA_BIN_DIR="/usr/local/bin"
    SUDO_PREFIX="sudo"
  else
    NOVA_BIN_DIR="$HOME/.local/bin"
    SUDO_PREFIX=""
  fi
}

resolve_url() {
  local repo_url="https://github.com/${repo}"
  local asset="${binary_name}-${PLATFORM_OS}-${PLATFORM_ARCH}"
  local candidates=(
    "${repo_url}/releases/latest/download/${asset}"
    "${repo_url}/releases/latest/download/${asset}.tar.gz"
    "${repo_url}/releases/latest/download/${asset}.zip"
  )

  if [ -n "$manual_url" ]; then
    echo "$manual_url"
    return
  fi

  for url in "${candidates[@]}"; do
    if curl -fsSLI -o /dev/null "$url"; then
      echo "$url"
      return
    fi
  done

  local api_url="https://api.github.com/repos/${repo}/releases/latest"
  local browser_url
  browser_url="$(curl -fsSL "$api_url" | awk -F'"' '/"browser_download_url"/ {print $4}' | grep -E \"/${binary_name}-${PLATFORM_OS}-${PLATFORM_ARCH}($|[.])\" | head -n1 || true)"
  if [ -z "$browser_url" ]; then
    echo "未找到匹配平台的可下载 CLI（${repo}）"
    exit 1
  fi
  echo "$browser_url"
}

detect_default_repo() {
  local remote
  if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    remote="$(git config --get remote.origin.url || true)"
    if [ -n "$remote" ]; then
      if [[ "$remote" == git@github.com:* ]]; then
        remote="${remote#git@github.com:}"
      elif [[ "$remote" == http*://github.com/* ]]; then
        remote="${remote#*github.com/}"
      fi
      remote="${remote%.git}"
      if echo "$remote" | grep -q "/"; then
        echo "$remote"
        return
      fi
    fi
  fi
  echo "luaxlou/nova-run"
}

is_archive() {
  local file="$1"
  local magic
  magic="$(head -c 4 "$file" | od -An -t x1 | tr -d ' \n')"
  case "$magic" in
    1f8b*|504b0304|504b0506|504b0708) return 0 ;;
    *) return 1 ;;
  esac
}

main() {
  detect_platform
  choose_install_dir

  url="$(resolve_url)"
  echo "[nova] 下载 CLI: ${url}"

  tmp="$(mktemp)"
  trap 'rm -f "$tmp"' EXIT
  curl -fsSL --retry 3 --connect-timeout 8 --max-time 120 "$url" -o "$tmp"

  if is_archive "$tmp"; then
    echo "下载的是压缩包，当前不支持直接安装压缩包。"
    echo "请设置 NOVA_CLIENT_DOWNLOAD_URL 为直接二进制链接后重试。"
    exit 1
  fi

  chmod +x "$tmp"
  mkdir -p "$NOVA_BIN_DIR"
  if [ -n "$SUDO_PREFIX" ]; then
    $SUDO_PREFIX mv "$tmp" "${NOVA_BIN_DIR}/${binary_name}"
  else
    mv "$tmp" "${NOVA_BIN_DIR}/${binary_name}"
  fi
  echo "[nova] 已安装到：${NOVA_BIN_DIR}/${binary_name}"

  if [ -r "${NOVA_BIN_DIR}/${binary_name}" ] && [ -x "${NOVA_BIN_DIR}/${binary_name}" ]; then
    echo "[nova] 安装完成"
  else
    echo "[nova] 安装异常"
    exit 1
  fi
}

main "$@"
