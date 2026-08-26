# Scripts

## install-cli.sh

安装本机 Nova 客户端。它只下载并安装 `nova` 二进制，不做项目初始化。

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash
```

如果目标位置的 `nova` 已经与下载版本一致，脚本会提示无需更新并退出。需要强制重新安装时使用：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash -s -- --force
```

## install-agent.sh

在 Linux 服务器上安装 Nova Agent。安装完成后，脚本会输出 `nova init` 需要填写的 Nova Agent Endpoint 和访问令牌位置。

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-agent.sh | sudo bash
```

如果已经有明确的域名或公网地址，可以传入 `NOVA_AGENT_ENDPOINT`，让输出中的 Endpoint 可直接复制：

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-agent.sh | sudo env NOVA_AGENT_ENDPOINT=https://nova.example.com bash
```

## uninstall-agent.sh

从 Linux 服务器上卸载 Nova 运行端。
