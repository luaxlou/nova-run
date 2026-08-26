# Nova Run 生命周期规范

Nova 是按需运行的项目生命周期工具。本地模式为每个运行中的应用创建一个独立 supervisor；它不是常驻的全局 daemon，也不提供自动重启。

## 生命周期边界

Nova 提供两条路径：

1. 本地：`nova start|stop|restart|run|status [app|all]` 管理本机 supervisor，不连接 Agent。
2. 远端：`nova start|stop|restart|run|status [app|all] --remote` 通过 Nova Agent 控制和查询 systemd；远端 run 同样等价于 restart。

`deploy|logs|list|remove` 继续使用 Nova Agent。`nova logs` 在 v0.2.0 仍是远端命令；本地应用输出写入 start 结果中显示的 `output.log`。

Nova 不管理数据库、缓存、Ingress、反向代理、域名、TLS、负载均衡或服务发现。远端运行真相来自 systemd，远端日志真相来自 journald。

## 项目配置

单应用：

```yaml
app: example
start: exec ./bin/example
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
    start: exec ./bin/api
    build:
      commands:
        - go build -o dist/api ./cmd/api
    artifacts:
      - dist/api
    service:
      command: ./api
  web:
    start: exec npm run dev
    build:
      commands:
        - npm run build
    artifacts:
      - dist
```

顶层 `start` 是子应用默认值，子应用字段优先。start 禁止包含 NUL、回车或换行，继承调用 Nova 时的环境，并以 canonical `nova.yaml` 所在目录为工作目录。

`start` 必须是持续运行的前台命令，不能自行 daemonize。建议 shell 脚本最后使用 `exec`，使应用直接接管 shell 进程。旧的本地 `run: <command>` 和 `stop:` 字段均不再支持。

本地生命周期与部署独立校验：纯本地配置只需要 `start`；部署仍要求应用名和单一制品路径，并按现有规则校验构建与 service 配置。stop 和 status 只需要已配置的本地应用身份，因此即使 start 被临时移除，仍可查询或停止既有 supervisor。

## 本地命令语义

```text
nova start [app|all]
nova stop [app|all]
nova restart [app|all]
nova run [app|all]
nova status [app|all]
```

- `start` 创建 detached supervisor 并等待应用成功启动，然后立即返回；重复 start 是幂等成功。
- `stop` 请求 supervisor 停止完整应用进程组；先发 TERM，三秒仍未退出则发 KILL。已停止时幂等成功。
- `restart` 先停止全部选中应用，再启动全部选中应用。
- `run` 与 restart 完全等价，没有独立语义。
- `status` 通过锁和匹配的 Unix socket 查询活动 supervisor；锁空闲时读取最终 stopped/error 状态；从未启动显示 `not_started`；无法确认所有权时返回 `state=unknown` 错误。
- `all` 按 YAML 声明顺序执行，并在任何 stop/start 前校验全部 start 目标。批量启动中途失败时，只回滚本次新启动的 supervisor。
- 配置、启动或通信错误返回 1。

每个应用 supervisor 只监管一组本地进程，不自动重启。应用自然退出后，supervisor 原子保存退出码和最终状态，删除控制 socket、释放锁并退出。stdout/stderr 追加到用户缓存目录中的 `output.log`，start 输出该文件路径。

本地状态位于 `os.UserCacheDir()/nova/run/<project-hash>/<app-hash>`，项目目录不会产生 PID 或状态文件。Nova 使用独占 advisory lock、Unix socket 身份校验和随机 nonce 确认所有权；状态中的 PID 永远不会脱离锁与 socket 单独用于发信号。

## 远端命令语义

```text
nova start [app|all] --remote
nova stop [app|all] --remote
nova restart [app|all] --remote
nova run [app|all] --remote
nova status [app|all] --remote
```

`--remote` 可放在选择器前后。远端 start/stop/restart/status 保留现有 Agent API 行为，run 映射为 restart。没有 `--remote` 时，这五个生命周期命令不得读取 Endpoint、Token 或触发交互式初始化。

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

- 本地 supervisor（无 API） -> `nova start|stop|restart|run|status [app|all]`
- `PUT /v1/apps/{name}` -> `nova deploy [app|all]`
- `POST /v1/apps/{name}/start` -> `nova start [app|all] --remote`
- `POST /v1/apps/{name}/stop` -> `nova stop [app|all] --remote`
- `POST /v1/apps/{name}/restart` -> `nova restart|run [app|all] --remote`
- `GET /v1/apps/{name}/status` -> `nova status [app|all] --remote`
- `GET /v1/apps/{name}/logs` -> `nova logs [app|all]`
- `DELETE /v1/apps/{name}` -> `nova remove [app]`
- `GET /v1/apps` -> `nova list`

CLI 的 `[app]` 是当前项目 `nova.yaml` 中 `apps` 下的选择器，不是临时远端应用名。本地状态身份也始终使用选择器，不使用可选的远端 `app` 别名。

## 验证要求

- 配置测试覆盖 start-only、继承、默认选择器、`all` 顺序、身份查询、缺失字段和废弃字段。
- supervisor 测试覆盖锁、原子状态、readiness、重复 start、进程组清理、强制 KILL、自然退出、日志和批量回滚。
- CLI 测试覆盖默认本地、显式远端、`--remote` 位置、status 路由和 run/restart 等价。
- 完整测试、race、vet 与 Linux/macOS amd64/arm64 构建必须通过。

## v0.2.0 迁移

- 删除本地 `stop:` 字段；Nova supervisor 现在负责停止 start 产生的整个进程组。
- 确保 `start` 是持续运行且不自行后台化的前台命令；脚本应优先使用 `exec` 启动应用。
- 远端 start/stop/restart/status 脚本增加 `--remote`。
- `nova run` 继续是 restart 的精确别名：先 stop all，再 start all。
- 如果项目仍使用 v0.1.14 的后台 start 脚本，升级前必须改为前台运行，否则 Nova 只能记录脚本本身的退出状态。

## 规范维护说明

仓库根 `AGENTS.md` 要求读取 `openspec/AGENTS.md`，但当前仓库不存在该文件或 `openspec` 目录。本规范沿用仓库现有文档位置；在 OpenSpec 目录恢复前，不推测或创建未知模板。
