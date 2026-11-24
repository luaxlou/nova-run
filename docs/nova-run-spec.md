# Nova Run 生命周期规范

Nova 是按需运行的项目生命周期工具。本地模式为每个运行中的应用创建一个独立 supervisor；它不是常驻的全局 daemon，也不提供自动重启。

## 生命周期边界

Nova 提供两条路径：

1. 本地：`nova start|stop|restart|run|status [app|all]` 管理本机 supervisor，不连接 Agent。
2. 远端：`nova start|stop|restart|run|status [app|all] -r|--remote` 通过 Nova Agent 控制和查询 systemd；远端 run 同样等价于 restart。

`deploy|logs` 继续使用 Nova Agent。`nova logs` 仍是远端命令；本地应用输出写入 start 结果中显示的 `output.log`。应用清单与状态统一由 `nova status` 或 `nova status -r` 提供，不再保留单独的 `nova list`。

所有接受 `[app|all]` 的命令在省略选择器时都默认选择全部应用；只有显式指定应用选择器时才操作单个应用。`nova logs -f` 必须显式指定单个应用。

Nova 不管理数据库、缓存、Ingress、反向代理、域名、TLS、负载均衡或服务发现。远端运行真相来自 systemd，远端日志真相来自 journald。

`nova version`、`nova -v` 与 `nova --version` 输出 Release 构建时注入的 CLI 版本；`nova -h` 与 `nova --help` 输出帮助。本地开发构建默认显示 `dev`。客户端安装器先使用该版本与目标 Release 比较，仅在版本不同、旧客户端不支持版本查询或指定 `-f|--force` 时下载二进制。

生命周期命令的 `-r` 与 `--remote` 等价；日志命令的 `-f` 与 `--follow` 等价。短参数仅在对应命令上下文中解释。

## 项目配置

Nova Run 的项目模型与应用框架、编译器、制品格式和 Runtime 相互独立。`build.commands` 是项目拥有的普通命令；Nova 按顺序执行它们，但不解析外部产品清单、不加载外部 Runtime Provider，也不根据源码或文件扩展名猜测构建方式。`artifacts` 指向 Nova 要交付的一个外层文件或目录，目录内容对 Nova 而言是不透明的部署输入。

当项目明确采用 EV protocol 3 时，构建命令必须显式传入 Runtime 短名称，例如 `ev build api --runtime service-application` 或 `ev build web --runtime browser-application`。EV 从项目已经选择的精确 Capability 图中唯一解析 Runtime，并在 Artifact 中固定 Capability、精确版本和 Runtime 名称；Nova 不解析这些身份，只执行构建命令并封装声明的输出路径。`ev dev` 与源码 `ev start` 属于开发阶段，生产阶段不调用 EV。

单应用：

```yaml
project: example
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
project: example
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
      platform: linux-x64
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

`service.platform` 可选且当前只接受 `linux-x64`。它描述 Nova Agent 启动 standalone Service 前必须满足的目标主机平台，不赋予 Nova 解析编译器或外部 Artifact manifest 的责任。没有 `service.command` 的独立 selector 保持为静态 Artifact；项目可据此分别发布 Web 与 API。

Nova Run 0.2.15 首次提供该通用平台预检。采用 Runtime 短名称 Build/Start 合同的候选兼容组合为 EV Product Release 0.1.170、Runtime Patterns 0.8.0 与 Nova Run 0.2.16。Runtime Patterns 从 `main` 发布；这只是项目固定的命令和制品兼容组合，不表示 Nova 加载或解释 EV 或 Runtime Capability。

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
- `status` 通过锁和匹配的 Unix socket 查询活动 supervisor；锁空闲时读取最终 stopped/error 状态；从未启动显示 `not_started`；无法确认所有权时返回 `state=unknown` 错误。结果按 `APP STATE PID PORT AGE` 对齐输出；运行中应用的 `PORT` 来自应用进程及其子进程实际监听的 TCP 端口，`AGE` 从本次启动时间开始计算，非零退出码折叠为 `error(<code>)`。
- `all` 按 YAML 声明顺序执行，并在任何 stop/start 前校验全部 start 目标。批量启动中途失败时，只回滚本次新启动的 supervisor。
- 配置、启动或通信错误返回 1。

每个应用 supervisor 只监管一组本地进程，不自动重启。应用自然退出后，supervisor 原子保存退出码和最终状态，删除控制 socket、释放锁并退出。stdout/stderr 追加到用户缓存目录中的 `output.log`，start 输出该文件路径。

本地状态位于 `os.UserCacheDir()/nova/run/<project-hash>/<app-hash>`，项目目录不会产生 PID 或状态文件。Nova 使用独占 advisory lock、Unix socket 身份校验和随机 nonce 确认所有权；状态中的 PID 永远不会脱离锁与 socket 单独用于发信号。

## 远端命令语义

```text
nova start [app|all] [-r|--remote]
nova stop [app|all] [-r|--remote]
nova restart [app|all] [-r|--remote]
nova run [app|all] [-r|--remote]
nova status [app|all] [-r|--remote]
```

`-r|--remote` 可放在选择器前后。远端 start/stop/restart/status 保留现有 Agent API 行为，run 映射为 restart。远端 status 使用与本地相同的表格并增加 `VERSION` 列；Agent 未提供端口时 `PORT` 显示 `-`。没有远端参数时，这五个生命周期命令不得读取 Endpoint、Token 或触发交互式初始化。

Agent Endpoint 和访问令牌是本机 Nova CLI 的全局目标配置，默认保存在 `~/.nova/client.env`。不同项目复用同一个目标；`nova.yaml` 不保存 Endpoint 或令牌，顶层 `project` 也不参与凭据选择。服务器使用 `/etc/nova/token` 验证远端请求。环境中的 `NOVA_ENDPOINT` 与 `NOVA_TOKEN` 可以显式覆盖文件值，但不会改变项目配置。

## 制品与远端运行

`service.command` 仍是远端生产进程入口。发布目录型 Service Artifact 时，Nova 在通用外层包生成 `run` 和 `nova.app.yaml`，把项目声明的 Artifact 原样复制到 `artifact/`，并从该目录直接执行 standalone `service.command`；这里的制品文件名 `run` 与已移除的本地配置字段无关。该命令不得调用 `ev start`、Bun、包安装器或构建器。

Service Artifact 替换后，Agent 必须确认 systemd 进程连续稳定处于 `active/running`，再提交部署版本。`systemctl restart` 只表示请求被接受，不构成部署成功证据；进程立即退出、进入 `failed` 或 `auto-restart` 时，Agent 停止该服务的自动重试、返回部署失败，并保留未提交的 Artifact 供诊断和同版本重试。这个检查只观察通用进程状态，不解析或适配 `service.command` 所属的外部产品。

这层通用包装使要求内容闭包保持不变的第三方制品可以直接成为 `artifacts` 指向的目录，不需要项目增加另一条打包命令。Nova 不解析内部清单，也不改变内部文件；它只隔离自己的部署元数据并在声明的 Artifact 根执行项目给出的命令。这是两个独立系统通过命令和目录边界协作，不表示 Nova 理解或适配该 framework。

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
- `GET /v1/apps` 返回该 Agent 上的应用。

CLI 的 `[app]` 是当前项目 `nova.yaml` 中 `apps` 下的选择器，不是临时远端应用名。本地状态身份也始终使用选择器，不使用可选的远端 `app` 别名。

## Agent Plugin

Nova Run 同一产品版本还可以分发 Agent Plugin。Codex 与 Claude Code 的清单都指向仓库根目录同一份 `skills/`；Plugin 不增加第二套项目模型，也不改变本规范中的 CLI、Agent 或 API 语义。

`$nova:init` 根据现有项目事实建立或维护 `nova.yaml`，并且只在机器尚无全局目标、用户明确要求且已有安全凭据输入时调用 `nova init`。它不为各项目重复写 Endpoint，不执行构建或部署，不改变本地或远端进程，也不安装 Agent 或配置阿里云资源。

`$nova:doctor` 默认只读地检查 CLI 版本、项目配置、仓库事实和本地绑定。任何修复都必须先说明精确的本地改动并逐项获得授权；Doctor 不得隐式调用部署、生命周期变更、授权轮换、云控制面或其他外部写操作。远端只读检查也只有在用户要求相应诊断且本地绑定已经存在时才执行。

Plugin 清单、Marketplace、Skill 声明和根 `VERSION` 必须保持同一版本。Release 在发布二进制前运行全量测试和 Plugin 打包校验，并把一个平台无关的 `nova-run-plugin-<version>.tgz` 与各平台 CLI 二进制一起加入校验和。

## 验证要求

- 配置测试覆盖 start-only、继承、默认选择器、`all` 顺序、身份查询、缺失字段和废弃字段。
- supervisor 测试覆盖锁、原子状态、readiness、重复 start、进程组清理、强制 KILL、自然退出、日志和批量回滚。
- CLI 测试覆盖默认本地、显式远端、`--remote` 位置、status 路由和 run/restart 等价。
- Plugin 测试覆盖清单与 Skill 版本一致性、归档白名单、版本漂移拒绝，以及两项 Skill 的结构校验。
- 完整测试、race、vet 与 Linux/macOS amd64/arm64 构建必须通过。

## v0.2.0 迁移

- 删除本地 `stop:` 字段；Nova supervisor 现在负责停止 start 产生的整个进程组。
- 确保 `start` 是持续运行且不自行后台化的前台命令；脚本应优先使用 `exec` 启动应用。
- 远端 start/stop/restart/status 脚本增加 `--remote`。
- `nova run` 继续是 restart 的精确别名：先 stop all，再 start all。
- 如果项目仍使用 v0.1.14 的后台 start 脚本，升级前必须改为前台运行，否则 Nova 只能记录脚本本身的退出状态。

## 规范维护说明

仓库根 `AGENTS.md` 要求读取 `openspec/AGENTS.md`，但当前仓库不存在该文件或 `openspec` 目录。本规范沿用仓库现有文档位置；在 OpenSpec 目录恢复前，不推测或创建未知模板。
