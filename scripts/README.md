# Nova Run Scripts

本目录用于 nova-run 的项目与服务端脚本，均已切换为单二进制 `nova`。

## 当前建议脚本

- `install-agent.sh`：在 Linux 主机部署 `nova`，启动 `nova agent` 并注册 systemd 服务
- `uninstall-agent.sh`：卸载 `nova` 服务与二进制
- `install-cli.sh`：安装 `nova` CLI（下载匹配平台二进制到本机）

## 发布时流程（推荐）

1. 安装 CLI（curl，仅做安装）：
   - `curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash`
2. 在项目目录执行 `nova`（初始化）：
   - 首次运行会检测是否缺少本地连接配置，若缺失会进入交互式初始化
3. 本地构建并上传 `nova` Linux amd64 二进制：
   - `GOOS=linux GOARCH=amd64 go build -o dist/nova ./cmd/nova`
4. 在服务端执行 `install-agent.sh`（会安装并启动 `nova agent`）

说明：后续一切项目生命周期操作（deploy/start/stop/restart/status/logs）都通过 `nova` 进行，不需要再执行 `nova install`。
