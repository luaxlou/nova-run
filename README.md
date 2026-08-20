# nova-run

Nova 是一个单二进制应用管理工具，用于把项目发布到一台服务器，并管理它的运行生命周期。

## 安装 Nova 客户端

```bash
curl -fsSL https://raw.githubusercontent.com/luaxlou/nova-run/main/scripts/install-cli.sh | bash
```

这一步只做安装：下载当前系统对应的 `nova` 二进制，并安装为本机命令。

安装不会初始化项目，不会写项目配置，也不会询问 Endpoint 或 Token。

## 初始化项目

进入项目目录后执行：

```bash
nova
```

这一步只初始化当前项目。首次执行时，Nova 会进入交互式初始化，要求填写远端 Endpoint 和 Token。

初始化不会安装 Nova 客户端；如果本机还没有 `nova` 命令，请先完成上面的安装步骤。

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
scripts/install-agent.sh
```

服务器侧负责接收发布请求，并通过 systemd/journald 管理应用进程和日志。
