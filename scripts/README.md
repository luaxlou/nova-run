# Scripts

## package-plugin.sh

校验 `VERSION`、Codex/Claude Plugin 清单、Claude Marketplace 与两项产品 Skill 的版本一致性，然后生成平台无关的 `dist/nova-run-plugin-<version>.tgz`：

```bash
bash scripts/package-plugin.sh
```

归档只包含 Plugin 清单、Marketplace 入口、根 `skills/` 和 `docs/agent-plugin.md`，并规范化顺序、时间、所有者和权限，使相同内容得到相同制品。可以用 `NOVA_PLUGIN_OUTPUT_DIR` 指定输出目录；脚本不会安装 Plugin，也不会修改用户的 Codex、Claude Code 或 Nova 配置。

## install-cli.ps1

在 Windows PowerShell 中安装 Nova 客户端，并校验 Release 的 SHA256：

```powershell
irm https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.ps1 | iex
```

默认安装到当前用户的 `%LOCALAPPDATA%\Programs\Nova`，并加入用户 `PATH`。可以通过 `NOVA_VERSION`、`NOVA_INSTALL_DIR` 或脚本参数 `-Version`、`-InstallDir`、`-Force` 调整。

## install-cli.sh

安装本机 Nova 客户端。它只下载并安装 `nova` 二进制，不做项目初始化。

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash
```

脚本会先通过 `nova version` 比较本地与目标版本。版本一致时不会下载 Release 二进制，并提示无需更新。需要强制重新安装时使用：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash -s -- -f
```

`-f` 与 `--force` 等价。

## install-agent.sh

在 Linux 服务器上安装 Nova Agent。安装完成后，先执行 `sudo nova auth set <project>` 创建或覆盖单个项目授权，再把输出的项目 token 交给该项目中的 `nova init`。

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-agent.sh | sudo bash
```

如果已经有明确的域名或公网地址，可以传入 `NOVA_AGENT_ENDPOINT`，让输出中的 Endpoint 可直接复制：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-agent.sh | sudo env NOVA_AGENT_ENDPOINT=https://nova.example.com bash
```

## uninstall-agent.sh

从 Linux 服务器上卸载 Nova 运行端。
