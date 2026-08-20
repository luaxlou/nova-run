# nova-run

Nova 是一个单二进制应用管理工具，用于把项目发布到一台服务器，并管理它的运行生命周期。

## 安装 Nova 客户端

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash
```

这一步只做安装：下载当前系统对应的 `nova` 二进制，并安装为本机命令。

安装不会初始化项目，不会写项目配置，也不会询问 Nova Agent Endpoint 或访问令牌。

如果本机已经安装过 `nova`，安装脚本会优先更新当前 `PATH` 中正在使用的 `nova` 所在目录。之后也可以直接执行：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash
```

## 初始化项目

请先在目标 Linux 服务器上安装 Nova Agent。安装完成后，脚本会输出 `nova init` 需要填写的 Nova Agent Endpoint 和访问令牌。

进入项目目录后执行：

```bash
nova
```

也可以显式执行：

```bash
nova init
```

这一步只初始化当前项目。首次执行时，Nova 会进入交互式初始化，要求填写 Nova Agent Endpoint 和访问令牌。

Nova Agent Endpoint 指安装了 Nova Agent 的那台机器的访问地址，例如 `http://你的服务器IP:32102`。

初始化不会安装 Nova 客户端；如果本机还没有 `nova` 命令，请先完成上面的安装步骤。

## 给 AI 的提示词

当项目开发、构建和发布主要交给 AI 处理时，可以按任务阶段选择下面两类提示词。

### 安装和配置 Nova Run

当目标是让 AI 帮你安装 Nova Run 客户端、安装服务器 Agent，并完成首次连接配置时，使用这段：

```text
你正在协助我安装和配置 Nova Run。

请完成这些工作：
- 在本机安装 `nova` 客户端。
- 在目标 Linux 服务器上安装 Nova Agent。
- 确认服务器 `32102/tcp` 可以从本机访问。
- 获取 Nova Agent Endpoint 和访问令牌。
- 在本机项目目录中执行 `nova init`，把当前项目绑定到目标 Nova Agent。
- 用 `nova list` 验证本机客户端已经能连接服务器。

请遵守这些边界：
- Nova Agent 只安装在 Linux 服务器上。
- 不要把访问令牌、SSH 密钥或云厂商凭据写入仓库。
- 如果需要开放云安全组或防火墙，请先说明要开放的端口、协议和来源范围。
- 安装完成后，请说明客户端安装位置、Agent 服务状态、Endpoint、配置文件位置和验证结果。
```

### 在项目中引入 Nova Run

当 Nova Run 已经安装并且本机客户端已经配置完毕，只需要让 AI 在项目中使用 Nova Run 接管发布、运行、日志和生命周期时，使用这段：

```text
你正在协助维护一个已经配置好 Nova Run 的项目。

请假定本机 `nova` 客户端已经安装，且当前项目已经通过 `nova init` 绑定到目标 Nova Agent。

请把 Nova Run 当作这个项目的运行生命周期入口：
- 先识别项目的构建方式和产物目录。
- 如果项目还没有清晰的构建产物，请补齐最小必要的构建脚本或说明。
- 每次完成代码修改后，先运行项目测试和构建。
- 用 `nova deploy` 读取项目 `nova.yaml`，先执行 `build.commands`，再发布制品。
- 发布后用 `nova status [app]` 确认运行状态。
- 需要排查问题时，用 `nova logs [app]` 或 `nova logs [app] -f` 查看日志。
- 需要控制进程时，用 `nova start [app]`、`nova stop [app]`、`nova restart [app]`，其中 `[app]` 是 `nova.yaml` 中的可选子应用选择器。
- 需要移除应用时，用 `nova remove [app]`。

请遵守这些边界：
- Nova Run 只负责单机部署、进程生命周期和日志查看。
- Nova Run 不管理数据库、缓存、域名、Nginx、Ingress、负载均衡或服务发现。
- 不要把 Nova 访问令牌、服务器私密配置或临时密钥写入仓库。
- 如果发布失败，按顺序检查：本地构建产物、`nova` 目标配置、Agent 连通性、服务器 systemd 状态和 journald 日志。

执行任务时，请主动完成：代码修改、测试、构建、发布、状态检查和日志排查，并在最后说明使用过的 Nova 命令和结果。
```

## 管理项目

```bash
nova deploy
nova deploy [app]
nova start [app]
nova stop [app]
nova restart [app]
nova status [app]
nova logs [app] [-f]
nova list
nova remove [app]
```

## 项目部署配置

项目根目录使用 `nova.yaml` 声明部署方式。Nova 命令默认服务当前目录，不接收临时应用名或制品目录参数，而是读取这份配置。

```yaml
app: sbom-platform
build:
  commands:
    - npm run build
    - scripts/build-nova-artifact.sh
artifact:
  dir: .nova/artifact
```

多子应用项目可以在 `apps` 下声明可选目标：

```yaml
apps:
  backend:
    app: sbom-platform-backend
    build:
      commands:
        - go build -o .nova/backend/app ./cmd/api
    artifact:
      dir: .nova/backend
  worker:
    app: sbom-platform-worker
    build:
      commands:
        - go build -o .nova/worker/app ./cmd/worker
    artifact:
      dir: .nova/worker
```

之后使用：

```bash
nova deploy backend
nova restart worker
nova logs backend -f
```

## 可选制品清单

Artifact 可以包含 `nova.app.yaml`，用于描述制品和运行入口。Nova 仍然只上传 artifact、替换部署目录、执行 `run` 并管理进程生命周期；清单只用于发布前校验和发布后摘要，不描述路由、前端、后端、域名或 TLS。

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
  command: ./run
runtime:
  healthCommand: curl -fsS http://127.0.0.1:8080/healthz
```

部署后 Nova 会提示：

```text
artifact manifest:
  app: sbom-platform
  artifact files: run, sbom-api, config.yaml, dist
  process command: ./run
  health command: curl -fsS http://127.0.0.1:8080/healthz
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
Nova Agent initialized. Use these values when running 'nova init' on your project machine:
Nova Agent Endpoint: http://<this-server-ip-or-domain>:32102
Access token: sudo cat /etc/nova/token
```

如果已经有明确的域名或公网地址，可以安装时传入 `NOVA_AGENT_ENDPOINT`，让脚本输出可直接复制的 Endpoint：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-agent.sh | sudo env NOVA_AGENT_ENDPOINT=https://nova.example.com bash
```
