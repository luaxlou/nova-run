# Changelog

All notable changes to Nova Run will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- 修复 `nova-agent` API 路由层编译错误，补齐 `RenderJSON`/`RenderJSONError` 使用方式。
- 修正 `api.go` 中应用列表目录读取逻辑，避免 `[]DirEntry` 与 `[]string` 类型冲突。
- 规范 `internal/deploy/replace.go` 中 `EnsureRunBinary` 的导入与调用。
- 增加 `nova-agent` Linux 交付流程（可直接由本机交付至服务端）与本次上线执行记录。

### Changed
- `nova` 安装建议增加无 sudo 场景下的本地用户目录安装方案（用于开发机快速验证）。

### Fixed
- 修复 `nova-agent` 在服务器 `digitagent.cn` 首次启动报 `status=203/EXEC` 的架构不匹配问题：补充 Linux 目标编译产物。
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
