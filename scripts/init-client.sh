#!/usr/bin/env bash
set -euo pipefail

info() {
  echo "[nova] $*"
}

NOVA_REPO="${NOVA_GITHUB_REPO:-}"
if [ -z "$NOVA_REPO" ]; then
  echo "未设置 NOVA_GITHUB_REPO（例如 your-org/nova-run）。可按下列方式执行："
  echo "  NOVA_GITHUB_REPO=<owner>/<repo> bash -s < <(curl ...)"
  exit 1
fi
NOVA_BINARY_NAME="${NOVA_BINARY_NAME:-nova}"
INSTALL_DIR="${NOVA_INSTALL_DIR:-}"
DOWNLOAD_URL="${NOVA_CLIENT_DOWNLOAD_URL:-}"

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
    armv7*|armv6*) arch="armv7" ;;
    i386|i686) arch="386" ;;
    *) arch="" ;;
  esac

  if [ -z "$os" ] || [ -z "$arch" ]; then
    echo "不支持当前运行平台：$(uname -s)/$(uname -m)"
    return 1
  fi

  PLATFORM_OS="$os"
  PLATFORM_ARCH="$arch"
}

prompt_input() {
  local __var="$1"
  local __prompt="$2"
  local __default="$3"
  local __value

  if [ -r /dev/tty ]; then
    read -r -p "$__prompt" __value < /dev/tty
  else
    read -r -p "$__prompt" __value
  fi
  __value="${__value:-$__default}"
  printf -v "$__var" "%s" "$__value"
}

prompt_secret() {
  local __var="$1"
  local __prompt="$2"
  local __value

  if [ -r /dev/tty ]; then
    read -s -p "$__prompt" __value < /dev/tty
    echo
  else
    read -s -p "$__prompt" __value
    echo
  fi
  __value="${__value:-}"
  printf -v "$__var" "%s" "$__value"
}

download_cli_binary() {
  local out_file="$1"
  if [ -n "$DOWNLOAD_URL" ]; then
    info "使用手动指定下载链接：$DOWNLOAD_URL"
    curl -fsSL --retry 3 --connect-timeout 8 --max-time 60 "$DOWNLOAD_URL" -o "$out_file"
    return 0
  fi

  local asset="${NOVA_BINARY_NAME}-${PLATFORM_OS}-${PLATFORM_ARCH}"
  local repo_url="https://github.com/${NOVA_REPO}"
  local candidates=(
    "${repo_url}/releases/latest/download/${asset}"
    "${repo_url}/releases/latest/download/${asset}.tar.gz"
    "${repo_url}/releases/latest/download/${asset}.zip"
  )

  local url selected=""
  for url in "${candidates[@]}"; do
    if curl -fsSLI -o /dev/null -A "nova-bootstrap" "$url"; then
      selected="$url"
      break
    fi
  done

  if [ -z "$selected" ]; then
    local api_json api_url
    api_url="https://api.github.com/repos/${NOVA_REPO}/releases/latest"
    if api_json="$(curl -fsSL "$api_url" 2>/dev/null)"; then
      selected="$(printf '%s\n' "$api_json" \
        | awk -F'"' '/"browser_download_url"/ {print $4}' \
        | grep -E "/${NOVA_BINARY_NAME}-${PLATFORM_OS}-${PLATFORM_ARCH}($|[.])" \
        | head -n1)"
    fi
  fi

  if [ -z "$selected" ]; then
    echo "未找到匹配平台的可下载二进制（${NOVA_REPO}）。"
    echo "请设置 NOVA_CLIENT_DOWNLOAD_URL 后重试："
    echo "  NOVA_CLIENT_DOWNLOAD_URL=<url> bash -s < <(curl ...)"
    return 1
  fi

  info "下载 CLI：$selected"
  curl -fsSL --retry 3 --connect-timeout 8 --max-time 120 "$selected" -o "$out_file"
}

choose_install_dir() {
  if [ -n "$INSTALL_DIR" ]; then
    NOVA_BIN_DIR="$INSTALL_DIR"
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

write_config() {
  config_path="${1:?config_path missing}"
  endpoint="${2:?endpoint missing}"
  token="${3:?token missing}"

  mkdir -p "$(dirname "$config_path")"
  cat > "$config_path" <<ENV
export NOVA_AGENT_ENDPOINT="$endpoint"
export NOVA_AGENT_TOKEN="$token"
ENV
  chmod 600 "$config_path"
}

info "Nova 客户端初始化向导"
info "用于创建本机 nova 客户端的远端连接配置（支持交互式、curl 一键执行）"

if ! detect_platform; then
  exit 1
fi

if [ -n "${NOVA_AGENT_ENDPOINT:-}" ]; then
  endpoint="$NOVA_AGENT_ENDPOINT"
else
  prompt_input endpoint "Nova Agent 地址 [http://127.0.0.1:32102]: " "http://127.0.0.1:32102"
fi

if [ -n "${NOVA_AGENT_TOKEN:-}" ]; then
  token="$NOVA_AGENT_TOKEN"
else
  prompt_secret token "Nova Agent Token: "
fi

if [ -z "$token" ]; then
  echo "Token 不能为空。"
  exit 1
fi

if [ -n "${NOVA_CLIENT_ENV:-}" ]; then
  config_path="$NOVA_CLIENT_ENV"
else
  prompt_input config_path "配置文件路径 [${HOME}/.nova/client.env]: " "${HOME}/.nova/client.env"
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
tmp_bin="$tmpdir/nova"

choose_install_dir
mkdir -p "$NOVA_BIN_DIR"
download_cli_binary "$tmp_bin"
if [ -f "$tmp_bin" ] && command -v file >/dev/null 2>&1; then
  mime="$(file -b "$tmp_bin" | tr '[:upper:]' '[:lower:]')"
  if printf '%s\n' "$mime" | grep -q "gzip compressed\|zip archive"; then
    echo "下载的是压缩包，当前脚本仅支持直接二进制发布。请先解压后放置到 ${NOVA_BIN_DIR}/${NOVA_BINARY_NAME}，或设置 NOVA_CLIENT_DOWNLOAD_URL。"
    exit 1
  fi
fi

chmod +x "$tmp_bin"
if [ -n "${SUDO_PREFIX}" ]; then
  $SUDO_PREFIX mv "$tmp_bin" "${NOVA_BIN_DIR}/${NOVA_BINARY_NAME}"
else
  mv "$tmp_bin" "${NOVA_BIN_DIR}/${NOVA_BINARY_NAME}"
fi
[ -x "${NOVA_BIN_DIR}/${NOVA_BINARY_NAME}" ] || {
  info "安装失败：未检测到可执行文件 ${NOVA_BIN_DIR}/${NOVA_BINARY_NAME}"
  exit 1
}
info "CLI 已安装到：${NOVA_BIN_DIR}/${NOVA_BINARY_NAME}"

write_config "$config_path" "$endpoint" "$token"
info "配置已写入 $config_path"

if [ "${NOVA_BIN_DIR}" = "${HOME}/.local/bin" ]; then
  case ":$PATH:" in
    *"${NOVA_BIN_DIR}":*)
      ;;
    *)
      info "${NOVA_BIN_DIR} 未在 PATH，建议追加："
      info "  export PATH=\"\${PATH}:${NOVA_BIN_DIR}\""
      ;;
  esac
fi

shell_rc=""
if [ "${SHELL##*/}" = "zsh" ]; then
  shell_rc="$HOME/.zshrc"
elif [ "${SHELL##*/}" = "bash" ]; then
  shell_rc="$HOME/.bashrc"
else
  shell_rc="$HOME/.profile"
fi

read_input="n"
if [ -r /dev/tty ]; then
  prompt_input read_input "是否将配置自动加入 ${shell_rc}（y/N）? " "n"
else
  read_input="n"
fi

if [[ "$read_input" =~ ^([Yy]|[Yy][Ee][Ss])$ ]]; then
  if ! grep -Fq "source \"$config_path\"" "$shell_rc" 2>/dev/null; then
    {
      echo
      echo "# Nova client config"
      echo "source \"$config_path\""
    } >> "$shell_rc"
    info "已写入：source \"$config_path\" 到 ${shell_rc}"
  else
    info "已检测到配置文件已在 ${shell_rc} 中。"
  fi
  # shellcheck disable=SC1090
  source "$config_path"
fi

info "初始化完成，正在验证连接..."
if command -v curl >/dev/null 2>&1; then
  if ! curl -fsS "${endpoint}/health" | sed -n '1,1p'; then
    info "health check 失败，请确认 nova-agent 已部署且 token 与服务端一致。"
    exit 1
  fi
else
  info "未检测到 curl，已跳过远程连通性检查。"
fi

info "建议后续动作："
info "1) nova status <app>"
info "2) nova list"
info "3) nova logs <app> [-f]"
