# nova-run

Nova 是一个单二进制应用管理工具，用于把项目发布到一台服务器，并管理它的运行生命周期。

## 安装 Nova 客户端

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.ps1 | iex
```

Windows 客户端支持部署、日志、配置与带 `--remote` 的远端生命周期命令。本地 `start`、`stop`、`restart`、`run` 和 `status` 的 supervisor 目前仅支持 macOS / Linux；Nova Agent 仍只安装在 Linux 服务器上。

这一步只做安装：下载当前系统对应的 `nova` 二进制，并安装为本机命令。

安装不会初始化项目，不会写项目配置，也不会询问 Nova Agent Endpoint 或访问令牌。

## 更新本地 Nova 客户端

Nova 不提供 `nova update` 命令。更新本地客户端时，重新执行安装脚本：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash
```

如果本机已经安装过 `nova`，脚本会优先检查当前 `PATH` 中正在使用的 `nova` 所在目录。例如当前命令来自 `/opt/homebrew/bin/nova`，脚本会先执行 `nova version` 并与目标版本比较；版本一致时不会下载 Release 二进制。

需要强制重新安装时执行：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash -s -- -f
```

需要固定版本时，传入 `NOVA_VERSION`：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | NOVA_VERSION=v0.2.10 bash
```

## 安装 Nova Run Plugin

CLI 执行确定性的配置、构建和生命周期命令；Plugin 则让 Codex 或 Claude Code 理解怎样建立 Nova 项目，以及怎样在不部署、不改变进程或云资源的前提下检查问题。两者使用同一个产品版本，Plugin 从仓库根目录的同一份 `skills/` 提供 init 与 doctor 能力。Codex 通过 `$nova:init`、`$nova:doctor` 调用；Claude Code 使用 `/nova:init`、`/nova:doctor`。

Codex 可以像 EV 一样从 `digitagent` 的 Gitee Marketplace 安装：

```bash
codex plugin marketplace add git@gitee.com:digitagent/nova-run.git --ref v0.2.10
codex plugin add nova@nova
```

GitHub 镜像保留同一版本；无法使用 Gitee SSH 时，可以改用：

```bash
codex plugin marketplace add luaxlou/nova-run --ref v0.2.10
codex plugin add nova@nova
```

Claude Code：

```bash
claude plugin marketplace add luaxlou/nova-run
claude plugin install nova@nova
```

正式发布后，可以把 Marketplace 固定到对应的 `vX.Y.Z` tag；更新 Plugin 后请重新打开会话，让宿主加载新版本。Plugin 安装不会替代上面的 CLI 安装，也不会自动安装服务器 Agent 或配置阿里云。完整的分发结构和安全边界见 [Agent Plugin 说明](docs/agent-plugin.md)。

## 初始化项目

在 Codex 中使用 `$nova:init`，或在 Claude Code 中使用 `/nova:init`。它会先根据项目实际的构建入口、制品和运行方式建立或维护 `nova.yaml`。只有你明确要求且已有安全凭据输入时，它才继续写入本机项目绑定；它不会部署、修改进程、安装远端 Agent，或创建和调整阿里云资源。

如果不使用 Plugin，也可以按 [项目部署配置](#项目部署配置)手工编写 `nova.yaml`。现有 `nova init` 命令不会创建这份文件；它只在配置有效后，把已有 Nova Agent 的 Endpoint 和项目 token 绑定到当前项目：

```bash
nova init
```

凭证保存在系统用户配置目录的 `nova/projects/<project>.yaml`，不会写入仓库。项目名称来自 `nova.yaml` 顶层的 `project`；目标服务器上的项目授权仍由现有 `sudo nova auth set <project>` 管理，重复执行会覆盖旧授权。

初始化完成后可以只读确认绑定：

```bash
nova target list
```

要检查已有项目时，在 Codex 使用 `$nova:doctor`，或在 Claude Code 使用 `/nova:doctor`。Doctor 默认只读地核对 CLI 版本、`nova.yaml`、仓库事实和本地目标绑定；它会先解释具体问题与最小修复，只有得到对该项修改的明确授权后才改动本地文件。Doctor 不会把一次检查解释成部署、生命周期变更、凭据轮换或云资源操作的授权。

## 本地生命周期

在 `nova.yaml` 中配置持续运行的本地前台命令：

```yaml
start: exec ./bin/app
```

Nova 为每个运行中的应用创建一个 detached supervisor；没有应用运行时，不保留全局 daemon。`start` 通过 `sh -lc` 在 `nova.yaml` 所在目录执行并继承调用环境，但命令本身不能 daemonize。shell 脚本建议最后使用 `exec` 启动应用。

```bash
nova start                 # 启动本地 supervisor，应用就绪后立即返回
nova stop                  # TERM 完整进程组，三秒后仍未退出则 KILL
nova restart               # 先停止全部目标，再启动全部目标
nova run                   # 与 restart 完全等价
nova status                # 查询本地 supervisor 或最终退出状态
```

这些本地命令不依赖 `nova init`、Nova Agent Endpoint 或访问令牌。应用 stdout/stderr 会追加到 start 输出的 `output.log`。应用自然退出后，supervisor 保存最终状态和退出码并退出，不自动重启。

`nova status all` 会把状态压缩成对齐表格。`PORT` 来自运行中应用进程及其子进程实际监听的 TCP 端口；`AGE` 从本次启动时间开始计算，即使应用已经停止也会继续增长：

```text
APP      STATE     PID    PORT       AGE
api      running   12345  3000,8080  2m
worker   stopped   -      -          3h
jobs     error(7)  -      -          5d
```

远端 `nova status all --remote` 使用相同布局，额外显示 `VERSION`。当前 Agent 不返回端口，所以远端 `PORT` 为 `-`。

多应用项目可以分别配置：

```yaml
project: example
apps:
  api:
    start: exec ./bin/api
  web:
    start: exec npm run dev
```

`all` 按 YAML 声明顺序执行；restart/run 会先停止全部目标，再启动全部目标。Nova 会在变更任何进程前校验全部 start；批量启动中途失败时，只回滚本次新启动的 supervisor。

> **v0.2.0 迁移提示：** 删除本地 `stop:`；把会自行后台化的 start 改成持续运行的前台命令。旧的 `run: <command>` 仍不支持。远程生命周期和状态命令必须增加 `--remote`。

## 管理项目

```bash
nova run
nova run [app|all]
nova run [app|all] --remote
nova deploy
nova deploy [app|all]
nova start [app|all]
nova stop [app|all]
nova restart [app|all]
nova start [app|all] --remote
nova stop [app|all] --remote
nova restart [app|all] --remote
nova status [app|all]
nova status [app|all] --remote
nova logs [app|all] [-f]
nova version | -v | --version
nova -h | --help
```

常用参数短写：`-r = --remote`、`-f = --follow`（仅用于 `nova logs`）、`-v = --version`、`-h = --help`。

## 项目部署配置

项目根目录使用 `nova.yaml` 声明部署方式。Nova 命令默认服务当前目录，不接收临时应用名或制品路径参数，而是读取这份配置。

```yaml
project: sbom-platform
app: sbom-platform
start: exec ./bin/sbom-platform
build:
  commands:
    - npm run build
    - scripts/build-release.sh
artifacts:
  - dist/nova
service:
  command: ./app
```

多子应用项目可以在 `apps` 下声明可选目标：

```yaml
project: sbom-platform
apps:
  sbom-platform-backend:
    start: exec ./bin/backend
    build:
      commands:
        - scripts/build-backend-artifact.sh
    artifacts:
      - backend/dist/backend
    service:
      command: ./api
      env:
        CONFIG_PATH: ./config.yaml
  sbom-platform-worker:
    start: exec ./bin/worker
    build:
      commands:
        - scripts/build-worker-artifact.sh
    artifacts:
      - worker/dist/worker
    service:
      command: ./worker
```

之后使用：

```bash
nova deploy sbom-platform-backend
nova deploy all
nova restart --remote sbom-platform-worker
nova status all --remote
nova logs sbom-platform-backend -f
```

上面的 `build.commands` 完全由应用自己决定。Nova 只在部署前按顺序调用这些命令，并发布 `artifacts` 指向的制品路径；这个路径可以是目录，也可以是单个文件。`service` 是可选的；没有 `service.command` 的目标会作为静态制品发布。

Nova Run 与应用框架、编译器、制品格式和 Runtime 是相互独立的系统。Nova 不解析外部产品清单，也不加载其 Toolchain Adapter；项目自己的构建命令负责生成 Nova 要交付的路径。仓库里出现某种 framework manifest，并不表示 Nova 需要对该 framework 做原生适配。

例如，EV protocol 3 工程可以把 `ev check` 和 `ev build [app]` 直接写入 `build.commands`，也可以明确选择 `ev dev <app>` 或 `ev start <app> [--prod]` 作为被 Nova 调用的生命周期命令。这些命令的清单解析、Adapter 选择、Artifact 构建、开发和启动语义都属于 EV；Nova 只观察进程与退出状态并封装配置的路径。这是命令级共生，不是 Nova 对 EV 的原生适配，也不表示 Nova 取代 EV 的开发和启动入口。

`nova deploy` 只接受干净的 Git 工作区。每次成功部署都会把当前 Git HEAD 短 SHA 写入服务端部署元数据；`nova status --remote` 会展示该 `version`。当本地版本与服务端已部署版本一致时，Nova 返回 `already latest`，跳过构建和上传。

## 制品清单

当项目配置声明了 `service.command`，Nova 会在发布前生成包含 `run` 和 `nova.app.yaml` 的部署包。目录型 Service Artifact 会被原样放进 Nova 外层包的 `artifact/`，`run` 先进入该目录，再执行相对于项目所声明 Artifact 根的 `service.command`；项目不需要为 Nova 编写二次打包命令。Nova Agent 仍然只上传 artifact、替换部署目录、执行 `run` 并管理进程生命周期；清单只用于发布前校验和发布后摘要，不描述路由、前端、后端、域名或 TLS。

项目可以直接把禁止增加文件的不可变目录配置为 `artifacts`；Nova 的通用部署包装会保持它的内容闭包不变。例如最终部署布局是：

```text
deployment/
├── artifact/     # 项目构建产生、外部 Runtime 验证的不可变制品
├── run           # Nova 外层启动器
└── nova.app.yaml # Nova 外层清单
```

`service.command` 的工作目录是 `artifact/`，因此可以直接写该产品公开的生产入口，例如 `ev start api-server --prod`；具体命令、环境和内部目录仍由项目与该 Runtime 的公开合同决定。

示例：

```yaml
app: sbom-platform
artifact:
  files:
    - run
    - sbom-api
    - config.yaml
    - dist
process:
  command: ./app
```

部署后 Nova 会提示：

```text
artifact manifest:
  app: sbom-platform
  artifact files: run, sbom-api, config.yaml, dist
  process command: ./app
```

服务器 Web 入口、静态文件服务、反向代理和 TLS 属于服务器环境配置，不属于 Nova 生命周期模型。

## 安装服务器运行端

在 Linux 服务器上执行：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-agent.sh | sudo bash
```

服务器侧负责接收发布请求，并通过 systemd/journald 管理应用进程和日志。

安装完成后会看到类似输出：

```text
Nova Agent initialized. Create or replace one project's authorization with:
  sudo nova auth set <project>
Then use these values when running 'nova init' in that project:
Nova Agent Endpoint: http://<this-server-ip-or-domain>:32102
Project token: output of 'sudo nova auth set <project>'
```

如果已经有明确的域名或公网地址，可以安装时传入 `NOVA_AGENT_ENDPOINT`，让脚本输出可直接复制的 Endpoint：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-agent.sh | sudo env NOVA_AGENT_ENDPOINT=https://nova.example.com bash
```
