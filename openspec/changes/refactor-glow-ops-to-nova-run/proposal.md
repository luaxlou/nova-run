# Change: 将 glow-ops 重构为 nova-run

## Why
当前 `glow-ops` 正在承载控制面、资源绑定、配置中心、Ingress、状态持久化与进程编排，已与“轻量化单机应用生命周期管理”目标不匹配。  
本次变更把仓库从“运维控制面”重构为 `nova`/`nova-agent` 的纯生命周期工具：部署、启动、停止、重启、状态、日志、移除。

## What Changes
- 重置产品边界，移除对资源编排、配置中心、日志文件轮转、发布历史、版本回滚、数据库/缓存绑定、Ingress 管理、健康探测的依赖。
- 引入新的运行时真相模型：
  - 文件系统 `/var/lib/nova/apps` 是部署事实来源。
  - systemd 是运行事实来源（启动状态与生命周期）。
  - journald 是日志事实来源。
- 统一 API 到 `PUT /v1/apps/{name}`、`POST /v1/apps/{name}/start|stop|restart`、`GET /v1/apps/{name}/status|/logs`、`DELETE /v1/apps/{name}`、`GET /v1/apps`。
- 改为双二进制结构：
  - `nova`：本地 CLI（保留本地目标/本地 artifact 历史）
  - `nova-agent`：无状态服务端（仅执行固定动作，不做远程 shell）
- 作为第一阶段实施，新增 OpenSpec 变更提案与开发骨架（cmd + internal），用于下一步分阶段交付。

## Impact
- Affected specs: `app-management`, `process-governance`, `authentication`, `server-management`, `system-initialization`
- Affected code: 新建 `cmd/nova`, `cmd/nova-agent`, `internal`（最小骨架）
- Breaking: 是（API/能力边界与现有 `glow-ops` 命令/能力不兼容）

## Rollback
- 旧能力先保留在当前代码中，不在此次提交中主动删除；后续以阶段化 PR 方式逐步下线。

