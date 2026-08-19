## 1. 产品方案
- [x] 1.1 写入重构提案（`proposal.md`）并声明破坏性变更边界
- [x] 1.2 产出设计说明（`design.md`）和执行清单（`tasks.md`）
- [x] 1.3 定义最小 OpenSpec delta（`specs/*`）

## 2. 基础结构
- [x] 2.1 新建 `cmd/nova` CLI 入口（stateless 的本地命令壳）
- [x] 2.2 新建 `cmd/nova-agent` server 入口（HTTP 生命周期网关）
- [x] 2.3 新建 `internal/{artifact,deploy,agent,runtime,client}` 目录与最小接口
- [x] 2.4 新建 `scripts/install-agent.sh` 与 `scripts/uninstall-agent.sh` 基础脚本

## 3. 工程与文档对齐
- [x] 3.1 更新 `README.md` 为 `nova-run` 目标边界与命令预览
- [x] 3.2 更新 `.github/workflows/release.yml`（按 `nova`, `nova-agent` 打包）
- [x] 3.3 更新 `scripts/README.md` 与仓库入口命名对齐
- [ ] 3.4 与 OpenSpec 现有 specs 对齐并补齐验收场景（下一步）

