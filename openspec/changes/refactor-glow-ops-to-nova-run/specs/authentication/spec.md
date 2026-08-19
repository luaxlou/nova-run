## MODIFIED Requirements
### Requirement: 认证管理 (Auth Management)
系统 MUST 使用本地 context 管理远端目标（targets），并可配置 server/token。  
`nova` 为纯客户端状态承载，服务端仅保留最小鉴权头校验，不提供“远程 shell”类能力。

#### Scenario: 管理目标配置
- **WHEN** 用户执行 `nova target add/use/list`
- **THEN** CLI MUST 只在本机 `~/.nova/contexts` 读写配置
- **AND** 配置缺失时 SHOULD 引导交互式初始化

### Requirement: HTTP 管理面鉴权 (HTTP Management API Authentication)
系统 MUST 要求管理 API 包含 `Authorization: Bearer <token>`，仅允许上述固定生命周期端点访问。

#### Scenario: 鉴权成功
- **WHEN** 客户端调用 `/v1/apps/*` 且携带有效 token
- **THEN** 请求 MUST 继续处理

#### Scenario: 不支持远程命令执行
- **WHEN** 客户端尝试通过已允许路由之外的方法/路径执行命令
- **THEN** 服务端 MUST 拒绝并返回 404/405/403

### Requirement: 安装期写入默认连接 (Installer Seeded Auth)
安装脚本必须将 token 只保存到 `~/.nova`，不将任何业务配置注入应用实例。

#### Scenario: 客户端首配置
- **WHEN** 安装脚本完成
- **THEN** 用户本地 CLI SHOULD 可直接读取 target 与 token

## REMOVED Requirements
### Requirement: Installer Seeded Auth（Glow 特有字段）
Glow 命名、路径与 `~/.glow*` 配置结构不再保留。

#### Scenario: 命名迁移
- **WHEN** 工具首次初始化
- **THEN** 目录必须按 `~/.nova` 结构初始化

