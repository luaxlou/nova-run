# Nova Run 方案改进（本次对齐版）

本仓库将以“最小生命周期控制”作为第一性原则，去除历史遗留控制面能力，聚焦轻量化单机生命周期。

## 关键边界（最终）
1. `nova run|stop|restart|status|logs|remove` 只负责单机生命周期
2. `deploy` 只替换当前目录 `/var/lib/nova/apps/<name>`
3. 不管理数据库/缓存/Ingress/Service Discovery
4. 不保留发布历史
5. rollback = 本地重部署
6. 运行真相来自 systemd，日志真相来自 journald

## 制品清单

Artifact 可以携带 `nova.app.yaml` 描述制品和运行入口：

```yaml
app: example
artifact:
  files:
    - run
    - app
    - config.yaml
    - dist
process:
  command: ./run
runtime:
  healthCommand: ./run --health
```

Nova 对该清单只做三件事：

1. 发布前校验 `artifact.files` 声明的路径存在且位于 artifact 内。
2. 保持 systemd 只执行 artifact 根目录的 `run`。
3. 发布后打印制品和运行入口摘要。

Nova 不区分前端和后端，不描述路由，不创建、不修改、不 reload Caddy/Nginx，也不管理域名、TLS、安全组或云厂商解析记录。

## API/CLI 对应关系
- `PUT /v1/apps/{name}` -> `nova deploy [app]` 根据当前目录 `nova.yaml` 构建并发布
- `POST /v1/apps/{name}/start` -> `nova start [app]`
- `POST /v1/apps/{name}/stop` -> `nova stop [app]`
- `POST /v1/apps/{name}/restart` -> `nova restart [app]`
- `GET /v1/apps/{name}/status` -> `nova status [app]`
- `GET /v1/apps/{name}/logs` -> `nova logs [app]`
  - `?lines=<n>`（回放，默认 100）
  - `?follow=true`（流式，`nova logs [app] -f`）
- `DELETE /v1/apps/{name}` -> `nova remove [app]`
- `GET /v1/apps` -> `nova list`

CLI 的 `[app]` 不是远端应用名，而是当前项目 `nova.yaml` 中 `apps` 下的子应用选择器。省略时使用顶层默认应用。

## 变更记录（里程碑）

- `a5c2ecb`：实现 Agent 与 CLI 的 runtime 控制（start/stop/restart）、部署替换和状态/日志查询
- `176d777`：补齐 `nova logs -f` 端到端流式日志能力
