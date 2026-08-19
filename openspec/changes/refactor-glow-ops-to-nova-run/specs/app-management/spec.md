## MODIFIED Requirements
### Requirement: Start App
系统 MUST 能通过 `nova start <app>` 发起系统化的生命周期操作，并以 systemd 状态作为运行依据。  
启动能力仅面向已部署到本地目录的可执行 `run` 文件；任何 `deploy` 和生命周期行为均为显式命令，不做隐式编排。

#### Scenario: 成功启动已部署应用
- **WHEN** 客户端请求 `POST /v1/apps/{name}/start`
- **THEN** 服务端 MUST 调用 `systemctl start nova@{name}.service`
- **AND** 服务端 MUST 返回标准化状态（含状态码）
- **AND** 启动失败时 MUST 返回错误信息，不执行额外回滚或配置变更

### Requirement: 应用元数据登记 (Register App Metadata)
系统 MUST 将 `apply` 类能力从应用元数据登记收敛为“部署替换”行为。  
除非伴随 `nova deploy`，服务端不维护应用配置数据库。

#### Scenario: 部署替代元数据登记
- **WHEN** 客户端发送 `PUT /v1/apps/{name}`
- **THEN** 服务端 MUST 上传并替换应用 artifact 到 `/var/lib/nova/apps/{name}`
- **AND** 服务端 MUST 校验包内 `run` 可执行存在
- **AND** 服务端 MUST 更新完成后可由 `start/stop/status/logs` 获取生命周期结果

### Requirement: 删除应用 (Delete App)
系统 MUST 通过 `DELETE /v1/apps/{name}` 停止并移除应用目录。

#### Scenario: 删除应用
- **WHEN** 客户端发送 `DELETE /v1/apps/{name}`
- **THEN** 系统 MUST 停止 `nova@{name}.service`
- **AND** 系统 MUST 删除 `/var/lib/nova/apps/{name}` 目录
- **AND** 系统 MUST 从 `GET /v1/apps` 列表中消失

## REMOVED Requirements
### Requirement: 应用元数据登记 (Register App Metadata)
该需求不再保留。  
`glow apply -f app.yaml` 等注册式能力不再作为独立能力保留。

#### Scenario: 旧能力移除
- **WHEN** 用户尝试调用旧的应用配置登记类端点
- **THEN** 系统 SHOULD 返回不再支持该能力或 404/405

### Requirement: Application Binary Upload
`/apps/{name}/binary` 这类“仅二进制上传未替换部署上下文”的旧接口不再单独保留。  
上传与替换动作统一走 `PUT /v1/apps/{name}`。

#### Scenario: 上传即部署
- **WHEN** 客户端上传新 artifact
- **THEN** 服务端必须在部署后返回可被 `start` 的可运行目录

## ADDED Requirements
### Requirement: Artifact Driven Deployment
系统 MUST only support artifact-driven deployment.

#### Scenario: 基于 dist 目录部署
- **WHEN** 客户端执行 `nova deploy my-app ./dist`
- **THEN** CLI MUST 打包并上传 artifact
- **AND** 服务端 MUST 把其原子替换为 `/var/lib/nova/apps/my-app` 当前内容
- **AND** 服务端 MUST 校验 `run` 存在且可执行

