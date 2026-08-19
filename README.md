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
当前提交完成了方案改进与最小骨架，未包含完整上线实现；旧 `glow-ops` 功能代码保留以便迁移。请按 OpenSpec 变更逐步替换。

