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
├── scripts/
│   ├── install-agent.sh
│   └── uninstall-agent.sh
└── README.md
```

## 当前状态
当前提交已完成单机控制最小闭环：制品打包、部署替换、服务生命周期、状态、日志（含流式）与卸载安装脚本。

## 发布上线清单
- 目标机器运行 `scripts/install-agent.sh`
- 确认 `NOVA_AGENT_TOKEN` 与 `/etc/nova-agent/token` 保持一致
- 准备 artifact（包含 `run`）并执行 `nova deploy <app> <artifact_dir>`
- Linux 服务器发布时需先交付 Linux amd64 二进制：
  - `GOOS=linux GOARCH=amd64 go build -o dist/nova-agent ./cmd/nova-agent`
  - 将该产物上传至服务端，再用 `install-agent.sh` 安装。
- 快速验收命令：
  - `nova list`
  - `nova status <app>`
  - `nova logs <app> [-f]`
  - `curl -s http://127.0.0.1:32102/health`
- 通过后提交 Release Note 和版本标签
