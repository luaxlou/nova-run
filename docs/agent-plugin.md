# Nova Run Agent Plugin

Nova Run 以一个产品同时交付命令行工具和 Agent Plugin。命令行工具负责可重复执行的项目配置、连接检查与运行生命周期动作；Plugin 让 Codex、Claude Code 等宿主理解何时应该初始化 Nova 管理的部署项目、如何解释已有配置，以及怎样在不扩大授权范围的前提下诊断问题。两者共同使用同一份产品版本，不建立彼此独立的配置模型。

## 一个产品，两种执行表面

Nova CLI 是确定性的执行表面。项目文件解析、凭据存放、构建、部署和本地或远端生命周期管理都由 CLI 及其既有协议完成。Plugin 不复制这些实现，也不直接实现云接入、制品上传或进程管理。

Agent Skills 是判断和协作表面。它们先理解项目当前状态，再调用已有 CLI 或使用本地文件工具完成明确范围内的工作，并把事实、问题之间的关系和仍需用户决定的内容用自然语言说明。Skill 不维护第二份项目状态，也不能把尚未获得的部署或外部系统权限视为已经授权。

## 单一 Skill 来源

所有产品 Skills 只维护在仓库根目录的 `skills/` 中。Codex 的 `.codex-plugin/plugin.json` 与 Claude Code 的 `.claude-plugin/plugin.json` 都通过 `./skills/` 发布同一组能力；`.agents/plugins/marketplace.json` 和 `.claude-plugin/marketplace.json` 则把各自宿主的 Marketplace 入口解析到同一个仓库根目录。

Plugin 的稳定机器名是 `nova`，两项 Skill 的机器名是 `init` 与 `doctor`。Codex 因此使用 `$nova:init` 与 `$nova:doctor`，Claude Code 使用 `/nova:init` 与 `/nova:doctor`。这只是宿主调用符号的差异，不产生不同的 Skill 副本。

不要在 `.agents/skills/`、`.claude/skills/` 或其他宿主目录复制产品 Skills。重复副本会造成发现结果、行为和版本发生漂移，也会把仓库本地工具误当成 Nova Run 的正式产品能力。

## 安装与分发

宿主可以从带精确版本 tag 的 Nova Run Git 仓库安装 Plugin。Codex 读取 `.agents/plugins/marketplace.json` 和 `.codex-plugin/plugin.json`；Claude Code 读取 `.claude-plugin/marketplace.json` 和 `.claude-plugin/plugin.json`。两个 Marketplace 中的 `nova` 条目都从仓库根目录加载，因此同一个 tag 同时确定清单和 Skills。

发布构建还会生成 `dist/nova-run-plugin-<version>.tgz`。归档保留仓库相对路径，并包含 `.codex-plugin/`、`.claude-plugin/`、`.agents/plugins/marketplace.json`、`skills/` 和本说明。这个制品用于随 Nova Run Release 分发和离线核验；宿主通过原生 Plugin/Marketplace 机制安装，安装流程不应把其中的 Skill 手工复制到用户目录。

根 `VERSION`、两个 Plugin 清单、Claude Marketplace 以及每个 Skill 声明的 Nova Run Release 版本必须一致。准备发布时应一起更新这些版本；打包脚本会在生成归档前验证它们和归档内容，避免 CLI 与当前会话加载的指令来自不同发布。Plugin 更新后，如果宿主仍在使用旧会话，应先重新加载宿主，再宣称新版本已经生效。

## Init 的责任边界

`$nova:init` 用于在现有代码目录中建立或补全 Nova 管理的部署项目与本地配置。它应先检查项目事实和已有文件，说明准备建立的应用、构建产物、启动命令、目标绑定与凭据保存位置，并在写入前让用户确认仍有歧义或会覆盖既有内容的决定。访问令牌和其他秘密只进入 Nova 已有的用户配置位置，不得写入仓库、Skill 或对话结果。

Init 不执行部署，不创建或修改阿里云资源，不调整安全组或防火墙，不安装或重启远端 Agent，也不轮换服务器授权。Nova Run 已有的阿里云接入仍由现有产品能力负责；初始化 Skill 不重新配置云接入，也不把一次项目初始化解释为任何外部写操作的授权。

## Doctor 的责任边界

`$nova:doctor` 默认只读地检查 Nova CLI、项目配置及其相互关系，给出能够复核的具体诊断。若某项本地问题存在安全、明确且不会改变部署意图的修复方式，Doctor 应先列出将修改的文件和内容，只有在用户直接授权后才执行该项修复，并在完成后重新运行只读检查。

Doctor 不是部署、迁移或云资源管理入口。它不得隐式构建或发布制品，不得改变远端应用状态、阿里云资源、网络策略、服务器服务或凭据，也不得删除或覆盖无法明确归属的项目内容。只读的远端核验也应说明将访问的目标；一旦诊断需要外部写入或超出安全本地修复范围，就应停止并把它作为独立操作交给用户决定。

这两个 Skill 可以引导用户进入后续工作，但不能借由“初始化”或“修复”扩大当前任务。部署执行、远端生命周期变更和云资源操作始终需要单独、明确的请求。
