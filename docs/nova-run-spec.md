# Nova Run 生命周期规范

Nova 是无状态的项目生命周期命令工具，不是本地进程 supervisor 或基础设施控制面。

## 生命周期边界

Nova 提供两条路径：

1. 本地：`nova start|stop|restart|run [app|all]` 执行 `nova.yaml` 中的 shell 命令，不连接 Agent。
2. 远端：`nova start|stop|restart|run --remote [app|all]` 通过 Nova Agent 控制 systemd；`run --remote` 等价于远端 restart。

部署、状态和日志命令继续使用 Nova Agent：`deploy|status|logs|list|remove`。

Nova 不管理数据库、缓存、Ingress、反向代理、域名、TLS、负载均衡或服务发现。远端运行真相来自 systemd，远端日志真相来自 journald。

## 项目配置

单应用：

```yaml
app: example
start: scripts/start-local.sh
stop: scripts/stop-local.sh
build:
  commands:
    - go build -o dist/example ./cmd/example
artifacts:
  - dist/example
service:
  command: ./example
```

多应用：

```yaml
apps:
  api:
    start: scripts/start-api.sh
    stop: scripts/stop-api.sh
    build:
      commands:
        - go build -o dist/api ./cmd/api
    artifacts:
      - dist/api
    service:
      command: ./api
  web:
    start: scripts/start-web.sh
    stop: scripts/stop-web.sh
    build:
      commands:
        - npm run build
    artifacts:
      - dist
```

顶层 `start`/`stop` 是子应用默认值，子应用字段优先。命令禁止包含 NUL、回车或换行，继承 Nova 的环境与标准输入输出，并始终以 `nova.yaml` 所在目录为工作目录。

本地生命周期与部署独立校验：纯本地配置只需要 `start`/`stop`；部署仍要求应用名、构建命令和单一制品路径。旧的 `run: <command>` 字段不再支持。

## 本地命令语义

```text
nova start [app|all]
nova stop [app|all]
nova restart [app|all]
nova run [app|all]
```

- `start` 只执行配置的 start。
- `stop` 只执行配置的 stop。
- `restart` 依次执行 stop、start。
- `run` 与 restart 完全等价。
- stop 失败时不执行 start。
- 子命令退出码原样传播；配置或启动错误返回 1。
- `all` 按 YAML 声明顺序执行，并在运行任何命令前校验全部目标。restart/run 会先停止全部目标，再启动全部目标。

Nova 不保存 PID、锁、缓存状态或日志，不负责后台化、守护、监控或自动重启。持续运行的应用必须让自己的 start 命令委托给进程管理器或完成后台化后返回。Nova 等待的只是配置命令本身。

## 远端命令语义

```text
nova start --remote [app|all]
nova stop --remote [app|all]
nova restart --remote [app|all]
nova run --remote [app|all]
```

`--remote` 可放在选择器前后。远端 start/stop/restart 保留现有 Agent API 行为；run 映射为 restart。没有 `--remote` 时，生命周期命令不得读取 Endpoint、Token 或触发交互式初始化。

## 制品与远端运行

`service.command` 仍是远端生产进程入口。发布时 Nova 生成 artifact 根目录的 `run` 脚本和 `nova.app.yaml`；这里的制品文件名 `run` 与已移除的本地配置字段无关。

Artifact 清单示例：

```yaml
app: example
artifact:
  files:
    - run
    - app
process:
  command: ./run
runtime:
  healthCommand: ./run --health
```

## API/CLI 对应关系

- 本地执行（无 API） -> `nova start|stop|restart|run [app|all]`
- `PUT /v1/apps/{name}` -> `nova deploy [app|all]`
- `POST /v1/apps/{name}/start` -> `nova start --remote [app|all]`
- `POST /v1/apps/{name}/stop` -> `nova stop --remote [app|all]`
- `POST /v1/apps/{name}/restart` -> `nova restart|run --remote [app|all]`
- `GET /v1/apps/{name}/status` -> `nova status [app|all]`
- `GET /v1/apps/{name}/logs` -> `nova logs [app|all]`
- `DELETE /v1/apps/{name}` -> `nova remove [app]`
- `GET /v1/apps` -> `nova list`

CLI 的 `[app]` 是当前项目 `nova.yaml` 中 `apps` 下的选择器，不是临时远端应用名。

## 验证要求

- 配置测试覆盖顶层、继承、覆盖、默认选择器、`all` 顺序、缺失字段和非法字符。
- 执行测试覆盖工作目录、环境、标准输入输出、顺序、失败短路和退出码。
- CLI 测试覆盖默认本地、显式远端、`--remote` 位置和 run/restart 等价。
- 完整测试、race、vet 与 Linux/macOS amd64/arm64 构建必须通过。

## v0.1.14 迁移

- 将 `run: <command>` 替换为 `start:` 和 `stop:`。
- 确保 start 命令自身返回；Nova 不再拥有前台进程组。
- 远端 start/stop/restart 脚本增加 `--remote`。
- `nova run` 现在是无状态 restart 别名，不再是前台开发 supervisor。

## 规范维护说明

仓库根 `AGENTS.md` 要求读取 `openspec/AGENTS.md`，但当前仓库不存在该文件或 `openspec` 目录。本规范沿用仓库现有文档位置；在 OpenSpec 目录恢复前，不推测或创建未知模板。
