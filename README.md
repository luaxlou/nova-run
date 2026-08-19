# nova-run

核心认知：
- `curl` 命令用于安装 Nova 客户端本机二进制。
- `nova` 命令用于在项目目录执行初始化，并管理应用生命周期（发布、启动、停止、回滚、日志、状态等）。

`nova-run` 是一个面向单服务器、无状态、语言无关的应用生命周期管理工具（单一二进制 `nova`）。

## 关键能力（当前阶段）
- `nova deploy <app> <artifact_dir>`
- `nova start <app>`
- `nova stop <app>`
- `nova restart <app>`
- `nova status <app>`
- `nova logs <app> [-f]`
- `nova list`
- `nova remove <app>`
- `nova rollback <app>`
- `nova agent --listen :32102 --app-root /var/lib/nova/apps --token-file /etc/nova/token`

## 设计边界
1. `nova` 统一承载控制与服务端能力。
2. Deploy 只做应用替换与最小状态管理。
3. 运行真相来自 systemd，日志真相来自 journald。
4. 不保留应用发布历史。
5. 不管理数据库/缓存/Ingress/服务发现。

## API/CLI 对应关系
- `PUT /v1/apps/{name}` -> `nova deploy`
- `POST /v1/apps/{name}/start` -> `nova start`
- `POST /v1/apps/{name}/stop` -> `nova stop`
- `POST /v1/apps/{name}/restart` -> `nova restart`
- `GET /v1/apps/{name}/status` -> `nova status`
- `GET /v1/apps/{name}/logs` -> `nova logs`
- `DELETE /v1/apps/{name}` -> `nova remove`
- `GET /v1/apps` -> `nova list`

## AI 提示词（团队对外协作/运维机器人可直接复制）
```text
该项目采用 nova 单二进制模式：同一个 `nova` 二进制支持本地控制与远端 `nova agent`。
当前服务器：<your-domain>（Nova 监听端口 32102）。

请按下面顺序协助我：
1) 安装 CLI 到本机（curl）：
   curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash
2) 初始化（在项目目录执行 `nova`）：
   - 若未检测到配置，将自动进入交互式初始化，要求输入 Endpoint/Token 等
3) 服务端安装：Linux 端执行 `scripts/install-agent.sh`（会启动 `nova agent`）。
4) 发布上线：准备 artifact（含 `run`），执行 `nova deploy <app> <artifact_dir>`。
5) 验证：`nova list`、`nova status <app>`、`nova logs <app> [-f]`、`curl -s http://<your-domain>:32102/health`
6) 发布产物历史位于 GitHub Releases，供本地/服务器通过 curl 拉取二进制。
```

## 安装与初始化（严格分离）

```bash
# 安装 CLI（仅安装）
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash

# 初始化（在目标项目目录执行）
cd /path/to/项目目录
nova
```

`nova` 在项目目录执行时会先检查配置文件（`~/.nova/client.env`）是否存在。未检测到配置时会启动初始化向导：
- 远端访问地址（例如 `http://<your-domain>:32102`）
- 连接密钥（对应 `/etc/nova/token` 中内容）
- 写入约定配置文件 `~/.nova/client.env`
- 可选自动追加 `source ~/.nova/client.env` 到当前 shell 启动文件

## 部署上线清单
- 目标机器运行 `scripts/install-agent.sh`
- 确认 `/etc/nova/token` 与本地 `NOVA_TOKEN` 一致
- 准备 artifact（包含 `run`）并执行 `nova deploy <app> <artifact_dir>`
- 快速验收命令：
  - `nova list`
  - `nova status <app>`
  - `nova logs <app> [-f]`
  - `curl -s http://127.0.0.1:32102/health`
- 通过后提交 Release Note 和版本标签
