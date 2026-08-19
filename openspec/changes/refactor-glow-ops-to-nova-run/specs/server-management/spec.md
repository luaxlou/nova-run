## MODIFIED Requirements
### Requirement: 服务器信息查看 (Server Info)
系统 MUST 以单节点管理视角提供基础服务器部署真相（如 `nova-agent` 运行状态与数据目录），不管理数据库、资源池和网关信息。

#### Scenario: 查看服务端运行信息
- **WHEN** 用户执行 `nova target`/客户端命令查询 Agent 能力边界
- **THEN** 系统 SHOULD 返回 Agent 的运行端口、状态、认证方式及基础运行目录

## REMOVED Requirements
### Requirement: 删除已集成资源 (Remove Integrated Resource)
资源相关 remove 能力（mysql/redis/nginx 等）不再包含在 nova-run。

#### Scenario: 资源能力移除
- **WHEN** 用户请求删除资源绑定能力
- **THEN** 服务端 MUST 不再提供该命令/API

### Requirement: 自我更新 (Self Update)
服务端二进制自更新能力不纳入本阶段范围。

#### Scenario: 不提供服务端更新 API
- **WHEN** 用户尝试执行旧式版本更新 API
- **THEN** 服务端 MUST 返回该能力不可用

