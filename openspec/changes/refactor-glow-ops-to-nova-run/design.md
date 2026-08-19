## Context
仓库目标已从 `glow-ops` 的编排/治理平台改为单机应用生命周期工具。  
核心约束是“无状态 Agent + systemd 运行 + journald 日志 + 本地 artifact 驱动”。

## Goals / Non-Goals
### Goals
- 统一最小 API 与 CLI 命令。
- 把部署文件系统路径作为唯一的服务器真相。
- 让 server 只做固定生命周期动作，不做进程托管与配置编排外的行为。
- 明确把构建、版本历史、回滚（本地重部署）交还给客户端。

### Non-Goals
- 提供 resource 管理（MySQL/Redis/Ingress）。
- 提供健康探测、自动重启策略、日志持久化策略（轮转/清理）。
- 提供服务端可观测平台（监控/告警）能力。

## Decisions
- 决定保留 `gin` 仅作为 HTTP 层基础设施，不引入额外控制平面框架。
- 决定不再持久化应用元数据和版本历史；`/var/lib/nova/apps/<name>` 目录即为当前部署版本。
- 决定 `nova-agent` 使用固定模板单元 `nova@.service`，不按应用生成动态 service 文件。
- 决定认证仅用于“固定能力 API 限制”，不是 shell/命令透传能力。
- 决定在第一阶段先创建实现骨架与文档，再用任务清单驱动逐步替换旧实现。

## Risks / Trade-offs
- 风险：旧功能被移除可能导致现有用户迁移困难。  
  风险缓解：README 明确不兼容说明，并保留旧仓库归档策略。
- 风险：systemd/journald 依赖增加 Linux 绑定。  
  风险缓解：在 CLI 与 agent 上增加能力说明与错误提示。

## Migration Plan
1. 提交当前方案与骨架，形成“技术栈对齐”基线。
2. 下一个阶段将旧 API/处理器替换为新 API/命令，逐步删改旧目录。
3. 与发布脚本和文档同步，完成 `nova` 与 `nova-agent` 可交付版本。

## Open Questions
- 是否默认将本地 artifact 压缩格式固定为 `.tar.zst` 还是先用 `tar.gz` 过渡？
- rollback 在客户端是“纯重部署”命令时，最少保留多久历史是更稳妥的策略？

