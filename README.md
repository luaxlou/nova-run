# nova-run

Nova 是一个单二进制应用管理工具，用于把项目发布到一台服务器，并管理它的运行生命周期。

## 安装 Nova 客户端

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash
```

这一步只做安装：下载当前系统对应的 `nova` 二进制，并安装为本机命令。

安装不会初始化项目，不会写项目配置，也不会询问 Nova Agent Endpoint 或访问令牌。

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

当项目开发、构建和发布主要交给 AI 处理时，可以把下面这段提示词放进项目说明、任务描述或 AI 代码助手的上下文里：

```text
你正在协助维护一个需要通过 Nova 发布到单台 Linux 服务器的项目。

请把 Nova 当作项目的运行生命周期工具使用：
- 本机先安装 `nova` 客户端。
- 服务器先安装 Nova Agent，并保存安装脚本输出的 Nova Agent Endpoint 和访问令牌。
- 在项目根目录执行 `nova init`，把该项目绑定到目标 Nova Agent。
- 每次完成代码修改后，先按项目自己的方式构建产物，再执行 `nova deploy <app> <artifact_dir>` 发布。
- 发布后用 `nova status <app>` 确认运行状态，用 `nova logs <app>` 或 `nova logs <app> -f` 查看日志。
- 需要控制进程时使用 `nova start <app>`、`nova stop <app>`、`nova restart <app>`。

请遵守这些边界：
- Nova 只负责单机部署、进程生命周期和日志查看。
- Nova 不管理数据库、缓存、域名、Nginx、Ingress、负载均衡或服务发现。
- 如果发布失败，先检查本地构建产物、Nova Agent Endpoint、访问令牌、服务器 systemd 状态和 journald 日志。
- 不要在仓库里生成临时密钥、访问令牌或服务器私密配置；需要凭据时提示用户从安全位置提供。

执行任务时，请主动完成：代码修改、构建、发布、状态检查和日志排查，并在最后说明使用过的 Nova 命令和结果。
```

## 管理项目

```bash
nova deploy <app> <artifact_dir>
nova start <app>
nova stop <app>
nova restart <app>
nova status <app>
nova logs <app> [-f]
nova list
nova remove <app>
```

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
