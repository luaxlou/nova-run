# Nova Run 方案改进（本次对齐版）

本仓库将以“最小生命周期控制”作为第一性原则，去除 Glow 控制面遗留能力。

## 关键边界（最终）
1. `nova run|stop|restart|status|logs|remove` 只负责单机生命周期
2. `deploy` 只替换当前目录 `/var/lib/nova/apps/<name>`
3. 不管理数据库/缓存/Ingress/Service Discovery
4. 不保留发布历史
5. rollback = 本地重部署
6. 运行真相来自 systemd，日志真相来自 journald

## API/CLI 对应关系
- `PUT /v1/apps/{name}` -> `nova deploy`
- `POST /v1/apps/{name}/start` -> `nova start`
- `POST /v1/apps/{name}/stop` -> `nova stop`
- `POST /v1/apps/{name}/restart` -> `nova restart`
- `GET /v1/apps/{name}/status` -> `nova status`
- `GET /v1/apps/{name}/logs` -> `nova logs`
- `DELETE /v1/apps/{name}` -> `nova remove`
- `GET /v1/apps` -> `nova list`

