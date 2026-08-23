# Nova Run 生命周期规范

Nova 以“最小生命周期控制”为第一性原则，负责本地开发进程与单机部署进程的生命周期，不扩展为基础设施控制面。

## 生命周期边界

Nova 提供两条相互独立的运行路径：

1. 本地开发：`nova run [app|all]` 直接在当前项目中以前台进程运行开发命令。
2. 远端单机：`nova deploy|start|stop|restart|status|logs|remove` 通过 Nova Agent 管理部署和 systemd 服务。

两条路径共享 `nova.yaml` 中的应用选择器，但不共享运行命令：

- 本地开发只读取 `run`。
- 远端部署和运行只读取 `build`、`artifacts` 与 `service`。

`nova run` 不读取或检查 Nova Agent Endpoint、访问令牌或网络连通性，未执行 `nova init` 时也必须可用。

Nova 不管理数据库、缓存、Ingress、反向代理、域名、TLS、负载均衡或服务发现。远端部署只替换 `/var/lib/nova/apps/<name>` 的当前制品，不保留发布历史；rollback 仍表示本地重新部署。远端运行真相来自 systemd，远端日志真相来自 journald。

## 项目配置

单应用可在 `nova.yaml` 顶层直接声明本地运行命令：

```yaml
app: example
run: go run ./cmd/example
build:
  commands:
    - go build -o dist/example ./cmd/example
artifacts:
  - dist/example
service:
  command: ./example
```

多应用项目在子应用中声明各自的命令：

```yaml
apps:
  api:
    run: go run ./cmd/api
    build:
      commands:
        - go build -o dist/api ./cmd/api
    artifacts:
      - dist/api
    service:
      command: ./api
  web:
    run: npm run dev
    build:
      commands:
        - npm run build
    artifacts:
      - dist
```

顶层 `run` 是子应用的默认值；子应用自己的 `run` 优先。`run` 是单个非空 shell 命令，禁止包含 NUL、回车或换行，不包含环境变量、工作目录、依赖关系、重启策略或健康检查等额外模型。命令继承调用 Nova 时的环境，并始终以 `nova.yaml` 所在目录为工作目录。

本地运行和远端部署采用独立校验：

- `nova run` 只要求选中的应用存在有效的 `run`。
- `nova deploy` 继续要求有效的应用名、构建命令和单一制品路径。
- 纯本地项目可以只有 `run`，不需要 `app`、`build`、`artifacts` 或 `service`。
- 增加 `run` 不得放松现有部署命令的配置校验。

## `nova run` 命令语义

命令形式：

```text
nova run
nova run <app-selector>
nova run all
```

选择器语义与现有命令一致：

- 无参数时运行顶层默认应用；没有顶层默认应用时运行 `apps` 中第一个声明的应用。
- 指定选择器时运行对应的子应用。
- `all` 按声明顺序选择 `apps` 下的全部应用，并发启动；没有 `apps` 时运行顶层默认应用一次。
- 任一被选中的应用没有有效 `run` 时，命令在启动任何进程前失败，不静默跳过。

每条命令通过 `sh -lc` 启动，与现有构建命令一样要求本地环境提供 `sh`。单应用直接连接当前终端的 stdin、stdout 和 stderr。多应用不复用 stdin，启动前打印 `[selector] $ <command>`；子进程输出保持原样写入当前终端，不缓存、不持久化，也不纳入远端 `nova logs`。

## 进程生命周期

`nova run` 是前台命令，Nova 进程是本地开发进程组的所有者：

1. 选中多个应用时并发启动全部命令。
2. 任一命令退出后，Nova 立即请求其余命令退出，并以最先退出命令的结果结束。
3. 收到中断或终止信号时，Nova 将信号传播给全部子进程。
4. 子进程未在 3 秒宽限期内退出时，Nova 强制清理它们，避免遗留本地服务。
5. 单应用正常或异常退出时，Nova 保留该命令的退出语义；用户中断使用平台惯例的中断退出结果。

启动失败必须包含应用选择器、命令和底层错误。配置失败必须标明对应的 `nova.yaml` 字段。Nova 不提供后台模式、自动重启、文件监听、端口分配或本地日志存储；这些行为由配置的开发命令自行实现。

## 代码边界

- `internal/project` 保存 `run` 配置，并提供本地运行专用的加载、合并、选择和校验入口。现有部署加载与解析入口保留原有严格语义。
- 独立的本地运行组件负责命令启动、并发、信号传播、退出结果和进程清理。
- `cmd/nova` 只负责识别 `run`、解析选择器、输出用户提示并调用本地运行组件。
- CLI 必须在远端配置自动初始化之前分流 `run`，确保它完全离线可用。

## 验证要求

配置测试覆盖：

- 顶层 `run`。
- 子应用 `run` 及其对顶层值的覆盖和继承。
- 默认应用、显式选择器与 `all` 的声明顺序。
- 缺失、空白或包含 NUL、回车、换行的命令。
- 纯本地配置可运行，同时仍不可部署。
- 新字段不会改变现有部署校验。

进程测试覆盖：

- 单应用命令的标准输入输出及退出结果。
- 多应用并发启动。
- 任一进程退出后清理其余进程。
- 中断信号传播与超时强制清理。
- 某个选择器缺少 `run` 时不会启动部分环境。

CLI 测试覆盖未配置 Endpoint 和 Token 时仍可执行 `nova run`。README 同步记录命令用法、配置示例、本地与远端生命周期边界。

## 远端制品清单

Artifact 可以携带 `nova.app.yaml` 描述制品和生产运行入口：

```yaml
app: example
artifact:
  files:
    - run
    - app
    - config.yaml
    - dist
process:
  command: ./run
runtime:
  healthCommand: ./run --health
```

Nova 对该清单只做三件事：

1. 发布前校验 `artifact.files` 声明的路径存在且位于 artifact 内。
2. 保持 systemd 只执行 artifact 根目录的 `run`。
3. 发布后打印制品和生产运行入口摘要。

本地配置字段 `run` 不写入制品清单，也不会改变服务器的 systemd 启动命令。

## API/CLI 对应关系

- 本地执行（无 API） -> `nova run [app|all]`
- `PUT /v1/apps/{name}` -> `nova deploy [app|all]`
- `POST /v1/apps/{name}/start` -> `nova start [app|all]`
- `POST /v1/apps/{name}/stop` -> `nova stop [app|all]`
- `POST /v1/apps/{name}/restart` -> `nova restart [app|all]`
- `GET /v1/apps/{name}/status` -> `nova status [app|all]`
- `GET /v1/apps/{name}/logs` -> `nova logs [app|all]`
- `DELETE /v1/apps/{name}` -> `nova remove [app]`
- `GET /v1/apps` -> `nova list`

CLI 的 `[app]` 是当前项目 `nova.yaml` 中 `apps` 下的子应用选择器，不是临时传入的远端应用名。

## 变更记录

- `a5c2ecb`：实现 Agent 与 CLI 的远端 runtime 控制、部署替换和状态/日志查询。
- `176d777`：补齐 `nova logs -f` 端到端流式日志能力。
- 当前设计：增加与远端 Agent 解耦的本地前台开发生命周期 `nova run`。

## 规范维护说明

仓库根 `AGENTS.md` 要求读取 `openspec/AGENTS.md`，但当前仓库不存在该文件或 `openspec` 目录。本规范沿用仓库现有的 `docs/nova-run-spec.md`；在 OpenSpec 目录恢复前，不推测或创建未知的 OpenSpec 模板结构。
