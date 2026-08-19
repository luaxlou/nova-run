## MODIFIED Requirements
### Requirement: 进程监控 (Process Monitoring)
系统 MUST 只从 systemd / journald 查询运行态与日志，不再在服务端维护应用进程的内建采集或周期扫描任务。

#### Scenario: 状态来源为 systemd
- **WHEN** 用户执行 `GET /v1/apps/{name}/status`
- **THEN** 系统 MUST 基于 `systemctl`/`systemd` 返回 State、SubState、PID、Started、ExitCode
- **AND** 不允许从旧的应用数据库字段推断“运行中/停止”状态

### Requirement: 重启应用 (Restart App)
系统 MUST 将重启定义为明确调用 `stop` 后 `start`，不保留自动重启策略与用户外部配置。

#### Scenario: 明确重启
- **WHEN** 用户调用 `POST /v1/apps/{name}/restart`
- **THEN** 系统 MUST 先触发 stop，再触发 start
- **AND** 不允许执行除这两个动作外的额外进程策略变更

### Requirement: 日志轮转 (Log Rotation)
该需求不再由 nova-run 持有，系统 MUST 删除服务端日志轮转/清理能力。

#### Scenario: 日志轮转外置
- **WHEN** 用户查询日志
- **THEN** 系统 MUST 读取 journald
- **AND** 不在本服务内对文件日志进行轮转与清理

## REMOVED Requirements
### Requirement: 日志目录规范 (Log Directory Layout)
应用日志目录不再为服务端托管文件路径，日志一律来自 journald。

#### Scenario: journald 为日志事实来源
- **WHEN** `nova logs <name>` 被调用
- **THEN** 服务端 MUST 使用 `journalctl -u nova@{name}`

### Requirement: 日志自我清理 (Log Retention Cleanup)
本地日志清理能力不再属于 nova-server 职责。

#### Scenario: 删除清理任务
- **WHEN** 服务端启动完成
- **THEN** 系统 MUST NOT 启动任何历史日志清理定时任务

## ADDED Requirements
### Requirement: 日志统一接入 Journald
系统 MUST 通过 journald 统一提供 `nova logs` 输出（支持 `-f` 流式）。

#### Scenario: 流式日志
- **WHEN** CLI 调用 `nova logs my-app -f`
- **THEN** 服务端 MUST 返回 journald 实时输出（长连接或 SSE/流式）

