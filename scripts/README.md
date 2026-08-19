# Nova Run Scripts

本目录保留与 `nova-run` 相关的本地/服务端脚本。  
旧 `glow` 命令与历史脚本仍保留在仓库中用于平滑迁移，但新开发请优先使用 nova 系列脚本与二进制名。

## 当前建议脚本

- `install-agent.sh`：在 Linux 主机部署 `nova-agent`，安装 systemd 模板 `nova@.service`
- `uninstall-agent.sh`：卸载 `nova-agent` 与服务文件（保留 `/var/lib/nova`）

## 发布脚本（现有）

- `release.sh`：仍可用于版本发版，但请注意产物名称将按 `nova` / `nova-agent` 输出。

