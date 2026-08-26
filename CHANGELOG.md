# Changelog

All notable changes to Nova Run will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.2] - 2026-08-26

### Changed
- `deploy`、`start`、`stop`、`restart`、`run`、`status` 和 `logs` 未指定应用时，统一默认操作 `nova.yaml` 中的全部应用；显式指定应用时仍只操作该应用。
- `nova logs -f` 必须显式指定单个应用，避免隐式跟随多应用日志。

### Removed
- 删除 `nova remove` CLI 及对应客户端方法，应用移除不再作为 Nova Run 的命令能力提供。

## [0.2.1] - 2026-08-26

### Changed
- 本地与远端 `nova status [app|all]` 改为对齐表格，移除配置路径、完整时间戳、空退出码和 sub-state 等干扰信息。
- 本地运行中应用显示实际监听的 TCP 端口；多个端口会排序、去重，无法发现端口时显示 `-`。
- 状态增加 Kubernetes 风格的 `AGE` 列，按本次启动时间显示秒、分、小时或天；错误退出码合并为 `error(<code>)`。
- 远端状态保留 `VERSION` 列；当前 Agent 不提供端口信息，因此远端 `PORT` 显示 `-`。

## [0.2.0] - 2026-08-26

### Added
- 为每个本地应用增加独立的按需 supervisor；没有应用运行时，不保留全局 Nova daemon。
- `nova status [app|all]` 默认读取本地 supervisor 的可信运行状态，`status --remote` 保留 Nova Agent 状态查询。
- 本地运行状态、退出码和日志保存在用户缓存目录；`start` 返回时会输出对应的 `output.log` 路径。
- 使用独占锁、Unix socket、随机 nonce 和原子状态文件确认进程所有权，不根据 PID 文件单独发送信号。

### Changed
- **破坏性变更：** 本地 `start` 必须是持续运行的前台命令；Nova 会将它置于独立进程组并在后台监管。
- **破坏性变更：** 本地 `stop:` 配置已移除；`nova stop` 由 supervisor 依次发送 TERM，并在三秒后升级为 KILL。
- `nova restart` 与 `nova run` 仍完全等价：先停止全部选中应用，再按 YAML 声明顺序启动全部应用。
- 本地 start/stop/restart/run/status 都不读取 Agent Endpoint 或 Token；远端生命周期与状态操作必须显式增加 `--remote`。
- 应用自然退出后 supervisor 会保存最终状态并退出，不执行自动重启。

### Removed
- 删除 v0.1.14 的同步 `internal/localcommand` 执行器和项目自定义本地 stop 命令。

## [0.1.14] - 2026-08-25

### Added
- 在 `nova.yaml` 顶层及 `apps.<selector>` 中支持无状态的本地 `start`、`stop` 生命周期命令。
- 生命周期命令支持 `--remote`，显式选择原有 Nova Agent 操作。
- 修复 `nova-agent` API 路由层编译错误，补齐 `RenderJSON`/`RenderJSONError` 使用方式。
- 修正 `api.go` 中应用列表目录读取逻辑，避免 `[]DirEntry` 与 `[]string` 类型冲突。
- 规范 `internal/deploy/replace.go` 中 `EnsureRunBinary` 的导入与调用。
- 增加 `nova-agent` Linux 交付流程（可直接由本机交付至服务端）与本次上线执行记录。

### Changed
- **破坏性变更：** `nova start`、`nova stop` 和 `nova restart` 默认执行本地配置；控制远端服务必须增加 `--remote`。
- **破坏性变更：** `nova run` 现在与 `nova restart` 等价，固定执行 `stop` 后执行 `start`；旧的 `run: <command>` 配置不再支持。
- Nova 不再监督本地进程或保存生命周期状态。需要后台运行时，项目自己的 `start` 命令必须完成后台化并返回。
- `nova` 安装建议增加无 sudo 场景下的本地用户目录安装方案（用于开发机快速验证）。

### Removed
- 删除本地前台进程 supervisor、进程组清理和自动等待行为。

### Fixed
- 修复 `nova-agent` 在服务器 `<your-domain>` 首次启动报 `status=203/EXEC` 的架构不匹配问题：补充 Linux 目标编译产物。
- 修复并完成远端 `systemd` 服务上线（`nova-agent.service`）后 healthcheck 可用。

---

## 版本号说明

- **[Unreleased]** - 即将发布的变更
- **[1.0.0]** - 第一个稳定版本（待发布）

### 变更类型

- **Added** - 新增功能
- **Changed** - 功能变更
- **Deprecated** - 即将废弃的功能
- **Removed** - 已移除的功能
- **Fixed** - Bug 修复
- **Security** - 安全性修复
