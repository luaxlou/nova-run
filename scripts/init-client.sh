#!/usr/bin/env bash
set -euo pipefail

info() {
  echo "[nova] $*"
}

info "Nova 客户端初始化向导"
info "用于创建本机 nova 客户端的远端连接配置（交互式）"

read -r -p "Nova Agent 地址 [http://127.0.0.1:32102]: " endpoint
endpoint="${endpoint:-http://127.0.0.1:32102}"
read -s -p "Nova Agent Token: " token
echo
if [ -z "$token" ]; then
  echo "Token 不能为空。"
  exit 1
fi

read -r -p "配置文件路径 [${HOME}/.nova/client.env]: " config_path
config_path="${config_path:-${HOME}/.nova/client.env}"

mkdir -p "$(dirname "$config_path")"
cat > "$config_path" <<ENV
export NOVA_AGENT_ENDPOINT="$endpoint"
export NOVA_AGENT_TOKEN="$token"
ENV
chmod 600 "$config_path"
info "配置已写入 $config_path"

shell_rc=""
if [ "${SHELL##*/}" = "zsh" ]; then
  shell_rc="$HOME/.zshrc"
elif [ "${SHELL##*/}" = "bash" ]; then
  shell_rc="$HOME/.bashrc"
else
  shell_rc="$HOME/.profile"
fi

read -r -p "是否将配置自动加入 ${shell_rc}（y/N）? " auto_load
if [[ "$auto_load" =~ ^([Yy]|[Yy][Ee][Ss])$ ]]; then
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
