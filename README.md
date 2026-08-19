# nova-run

`nova-run` 是一个面向单服务器、无状态、语言无关的应用生命周期管理工具（`nova` + `nova-agent`）。

## 关键能力（当前阶段）
- `nova deploy <app> <artifact_dir>`
- `nova start <app>`
- `nova stop <app>`
- `nova restart <app>`
- `nova status <app>`
- `nova logs <app> [-f]`
- `nova list`
- `nova remove <app>`

### 本地与远端分工
- `nova`（本地）：构建产物、目标切换、本地 artifact 历史（如 `~/.nova/artifacts`）与 rollback 重部署。
- `nova-agent`（远端）：监听固定 API，做原子替换、systemd 控制、journald 读取，不维护数据库。

### 设计原则
1. Agent 是无状态的（`State = 0`）。
2. Build 在本地执行。
3. Artifact 约定 `run`。
4. systemd 是 runtime。
5. journald 是日志系统。
6. 不保留服务端版本历史。

## API 形态（v1）
- `PUT /v1/apps/{name}`
- `DELETE /v1/apps/{name}`
- `POST /v1/apps/{name}/start|stop|restart`
- `GET /v1/apps`
- `GET /v1/apps/{name}/status`
- `GET /v1/apps/{name}/logs`
  - `?lines=<n>`：返回最近日志行数（默认 100）
  - `?follow=true`：返回实时日志流（配合 `nova logs <app> -f`）

## 目录预期（建议）
```text
nova-run/
├── cmd/
│   ├── nova/
│   │   └── main.go
│   └── nova-agent/
│       └── main.go
├── internal/
│   ├── agent/
│   ├── artifact/
│   ├── client/
│   ├── deploy/
│   └── runtime/
├── pkg/
│   └── api/
├── scripts/
│   ├── init-client.sh
│   ├── install-agent.sh
│   └── uninstall-agent.sh
├── docs/
│   └── nova-run-spec.md
└── README.md
```

说明：如果你看到 `glow` 命名的运维文档或脚本，均应按本仓库 nova-run 约定优先处理，本 README 为一次性来源。

## AI 提示词（团队对外协作/运维机器人可直接复制）
```text
该项目已切换为 nova-run（nova + nova-agent）架构，不再走 glow-ops 历史控制面。
当前服务器：<your-domain>（nova-agent 监听端口 32102）。
请按下面顺序协助我发布并运维：
1) 本地安装客户端：`go build -o dist/nova ./cmd/nova`，将 `dist/nova` 放入 PATH。
2) 服务端部署：在 Linux 端运行 `scripts/install-agent.sh`。
3) 本地客户端初始化：执行 `bash scripts/init-client.sh`，交互输入:
   - 远端访问地址（例如 `http://<your-domain>:32102`）
   - 与服务端一致的 token（即 `/etc/nova-agent/token` 中内容）
4) 本地发布上线：准备 artifact（含 `run`），执行 `nova deploy <app> <artifact_dir>`。
5) 验证：`nova list`、`nova status <app>`、`nova logs <app> [-f]`，以及 `curl -s http://<your-domain>:32102/health`。
```

## 本地客户端初始化（交互式）

```bash
bash scripts/init-client.sh
```

脚本会依次要求：
- 远端访问地址（例如 `http://<your-domain>:32102`）
- 服务端连接密钥（与 `/etc/nova-agent/token` 一致）

初始化会把配置写入 `~/.nova/client.env`，并可选自动追加 `source ~/.nova/client.env` 到当前 shell 的启动文件。

## 发布上线清单
- 目标机器运行 `scripts/install-agent.sh`
- 确认 `/etc/nova-agent/token` 与本地 `NOVA_AGENT_TOKEN` 一致
- 准备 artifact（包含 `run`）并执行 `nova deploy <app> <artifact_dir>`
- Linux 服务器发布时先交付 Linux amd64 二进制：
  - `GOOS=linux GOARCH=amd64 go build -o dist/nova-agent ./cmd/nova-agent`
  - 将该产物上传至服务端，再用 `install-agent.sh` 安装。
- 快速验收命令：
  - `nova list`
  - `nova status <app>`
  - `nova logs <app> [-f]`
  - `curl -s http://127.0.0.1:32102/health`
- 通过后提交 Release Note 和版本标签
