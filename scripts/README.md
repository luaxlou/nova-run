# Nova Run Scripts

本目录保留 nova-run 的本地/服务端脚本。

## 当前建议脚本

- `install-agent.sh`：在 Linux 主机部署 `nova-agent`，安装服务 `nova-agent.service`
- `uninstall-agent.sh`：卸载 `nova-agent` 与服务文件（保留 `/var/lib/nova`）
- `init-client.sh`：初始化本地客户端连接（交互式）

## 发布时流程（推荐）

1. 本地构建并上传 `nova-agent` Linux amd64 二进制：
   - `GOOS=linux GOARCH=amd64 go build -o dist/nova-agent ./cmd/nova-agent`
2. 在服务端执行 `install-agent.sh`
3. 本地执行 `bash scripts/init-client.sh` 完成交互式配置
