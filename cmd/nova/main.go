package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/luaxlou/glow-ops/internal/agent"
	"github.com/luaxlou/glow-ops/internal/artifact"
	"github.com/luaxlou/glow-ops/internal/client"
	"github.com/luaxlou/glow-ops/internal/localcommand"
	"github.com/luaxlou/glow-ops/internal/project"
	novaruntime "github.com/luaxlou/glow-ops/internal/runtime"
)

func usage() {
	fmt.Print(`nova (Nova Run CLI)

Usage:
  nova                # 无参数时会检查本地配置，不存在则进入交互式初始化
  nova init           # 初始化本机 CLI 要连接的发布目标
  nova agent --listen :32102 --app-root /var/lib/nova/apps --token-file /etc/nova/token
  nova run [app|all] [--remote]      # 默认在本地执行 stop + start；--remote 操作 Nova Agent
  nova deploy [app|all]   # 读取当前目录 nova.yaml，执行构建并发布；省略 app 时默认第一个
  nova start [app|all] [--remote]
  nova stop [app|all] [--remote]
  nova restart [app|all] [--remote]
  nova status [app|all]
  nova logs [app|all] [-f]
  nova list
  nova remove [app]

Local convenience:
  nova rollback [app]
  nova target add <name> --url <url> --token <token>
  nova target use <name>
  nova target list
`)
}

func main() {
	args := os.Args
	if len(args) >= 2 && isHelp(args[1]) {
		usage()
		return
	}
	if len(args) >= 2 && isLifecycleCommand(args[1]) {
		action := args[1]
		parsed, err := parseLifecycleArgs(args[2:])
		if err == nil && parsed.Remote {
			err = autoBootstrapRuntimeConfig("remote-lifecycle")
			if err == nil {
				var targets []project.Target
				targets, err = loadTargetsFromArgs(optionalArg(parsed.Selector))
				if err == nil {
					err = runConfiguredRemoteLifecycle(context.Background(), client.NewClient(), action, targets)
				}
			}
		} else if err == nil {
			err = runConfiguredLocalLifecycle(context.Background(), ".", action, parsed, localcommand.Streams{
				Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
			})
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s failed: %v\n", action, err)
			os.Exit(cliExitCode(err))
		}
		return
	}
	if len(args) >= 2 {
		if err := autoBootstrapRuntimeConfig(args[1]); err != nil {
			fmt.Printf("bootstrap failed: %v\n", err)
			os.Exit(1)
		}
	}

	if len(args) < 2 {
		if err := autoBootstrapRuntimeConfig(""); err != nil {
			fmt.Printf("bootstrap failed: %v\n", err)
			os.Exit(1)
		}
		usage()
		return
	}

	cmd := args[1]
	rest := args[2:]
	ctx := context.Background()
	cli := client.NewClient()

	switch cmd {
	case "init":
		if err := bootstrapRuntimeConfig(); err != nil {
			fmt.Printf("init failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("initialized")
	case "agent":
		if err := runAgent(rest); err != nil {
			fmt.Printf("agent failed: %v\n", err)
			os.Exit(1)
		}
	case "deploy":
		if err := runConfiguredDeploy(ctx, cli, rest); err != nil {
			fmt.Printf("deploy failed: %v\n", err)
			os.Exit(1)
		}
	case "status":
		targets, err := loadTargetsFromArgs(rest)
		if err != nil {
			fmt.Printf("status failed: %v\n", err)
			os.Exit(1)
		}
		for _, target := range targets {
			status, err := cli.Status(ctx, target.App)
			if err != nil {
				fmt.Printf("status failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(status)
		}
	case "logs":
		targets, follow, err := loadLogTargetsFromArgs(rest)
		if err != nil {
			fmt.Printf("logs failed: %v\n", err)
			os.Exit(1)
		}
		if follow && len(targets) > 1 {
			fmt.Println("logs failed: logs all -f is not supported; choose one app")
			os.Exit(1)
		}
		if follow {
			if err := cli.LogsStream(ctx, targets[0].App, os.Stdout); err != nil {
				fmt.Printf("logs failed: %v\n", err)
				os.Exit(1)
			}
		} else {
			for _, target := range targets {
				if len(targets) > 1 {
					fmt.Printf("==> %s <==\n", target.App)
				}
				stream, err := cli.Logs(ctx, target.App, follow)
				if err != nil {
					fmt.Printf("logs failed: %v\n", err)
					os.Exit(1)
				}
				for _, line := range stream {
					fmt.Println(line)
				}
			}
		}
	case "list":
		list, err := cli.List(ctx)
		if err != nil {
			fmt.Printf("list failed: %v\n", err)
			os.Exit(1)
		}
		for _, item := range list {
			fmt.Println(item)
		}
	case "remove":
		target, err := loadTargetFromArgs(rest)
		if err != nil {
			fmt.Printf("remove failed: %v\n", err)
			os.Exit(1)
		}
		if err := cli.Remove(ctx, target.App); err != nil {
			fmt.Printf("remove failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("removed")
	case "rollback":
		fmt.Println("rollback is a local operation; not implemented in this milestone")
	case "target":
		if len(rest) == 0 {
			usage()
			os.Exit(1)
		}
		switch rest[0] {
		case "add":
			if err := runTargetAdd(rest[1:]); err != nil {
				fmt.Printf("target add failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("target saved")
		case "use":
			fmt.Println("target use: single-target mode uses the active config file; run `nova init` or `nova target add <name> --url <url> --token <token>` to change it")
		case "list":
			if err := runTargetList(); err != nil {
				fmt.Printf("target list failed: %v\n", err)
				os.Exit(1)
			}
		default:
			usage()
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func runConfiguredDeploy(ctx context.Context, cli *client.Client, args []string) error {
	targets, err := loadTargetsFromArgs(args)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := deployTarget(ctx, cli, target); err != nil {
			return err
		}
	}
	return nil
}

type lifecycleArgs struct {
	Selector string
	Remote   bool
}

func isLifecycleCommand(action string) bool {
	switch action {
	case "start", "stop", "restart", "run":
		return true
	default:
		return false
	}
}

func parseLifecycleArgs(args []string) (lifecycleArgs, error) {
	var parsed lifecycleArgs
	for _, arg := range args {
		switch {
		case arg == "--remote":
			if parsed.Remote {
				return lifecycleArgs{}, fmt.Errorf("--remote may only be specified once")
			}
			parsed.Remote = true
		case strings.HasPrefix(arg, "-"):
			return lifecycleArgs{}, fmt.Errorf("unknown lifecycle argument: %s", arg)
		case parsed.Selector != "":
			return lifecycleArgs{}, fmt.Errorf("expected at most one configured app selector")
		case strings.TrimSpace(arg) == "":
			return lifecycleArgs{}, fmt.Errorf("configured app selector must not be empty")
		default:
			parsed.Selector = arg
		}
	}
	return parsed, nil
}

func lifecycleActions(action string) ([]project.LifecycleAction, error) {
	switch action {
	case "start":
		return []project.LifecycleAction{project.ActionStart}, nil
	case "stop":
		return []project.LifecycleAction{project.ActionStop}, nil
	case "restart", "run":
		return []project.LifecycleAction{project.ActionStop, project.ActionStart}, nil
	default:
		return nil, fmt.Errorf("unknown lifecycle action %q", action)
	}
}

func runConfiguredLocalLifecycle(ctx context.Context, dir, action string, parsed lifecycleArgs, streams localcommand.Streams) error {
	actions, err := lifecycleActions(action)
	if err != nil {
		return err
	}
	cfg, path, err := project.LoadForLifecycle(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(streams.Stdout, "project config: %s\n", path)

	var targets []project.LifecycleTarget
	if parsed.Selector == "all" {
		targets, err = project.ResolveAllLifecycles(cfg, actions...)
	} else {
		var target project.LifecycleTarget
		target, err = project.ResolveLifecycle(cfg, parsed.Selector, actions...)
		targets = []project.LifecycleTarget{target}
	}
	if err != nil {
		return err
	}

	commands := make([]localcommand.Command, 0, len(targets)*len(actions))
	for _, lifecycleAction := range actions {
		for _, target := range targets {
			command := target.Start
			if lifecycleAction == project.ActionStop {
				command = target.Stop
			}
			commands = append(commands, localcommand.Command{
				Target: target.Name, Action: string(lifecycleAction), ShellCommand: command,
			})
		}
	}
	return localcommand.Run(ctx, commands, filepath.Dir(path), streams)
}

type remoteLifecycleClient interface {
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Restart(context.Context, string) error
}

func runConfiguredRemoteLifecycle(ctx context.Context, cli remoteLifecycleClient, action string, targets []project.Target) error {
	for _, target := range targets {
		var err error
		switch action {
		case "start":
			err = cli.Start(ctx, target.App)
		case "stop":
			err = cli.Stop(ctx, target.App)
		case "restart", "run":
			err = cli.Restart(ctx, target.App)
		default:
			return fmt.Errorf("unknown remote lifecycle action %q", action)
		}
		if err != nil {
			return err
		}
		fmt.Printf("%s %s command sent\n", target.App, action)
	}
	return nil
}

func cliExitCode(err error) int {
	var exitCoder interface{ ExitCode() int }
	if errors.As(err, &exitCoder) && exitCoder.ExitCode() > 0 {
		return exitCoder.ExitCode()
	}
	return 1
}

func deployTarget(ctx context.Context, cli *client.Client, target project.Target) error {
	if len(target.Build.Commands) == 0 {
		return fmt.Errorf("no build commands configured for %s", targetLabel(target))
	}
	version, err := gitDeploymentVersion(".")
	if err != nil {
		return err
	}
	status, err := cli.AppStatus(ctx, target.App)
	if err != nil {
		return err
	}
	if strings.TrimSpace(status.Version) == version {
		fmt.Printf("already latest app=%s version=%s\n", target.App, version)
		return nil
	}
	if err := runShellCommands(ctx, target.Build.Commands); err != nil {
		return err
	}
	artifactPath, cleanup, err := prepareDeployArtifact(target.Artifacts[0], target)
	if err != nil {
		return err
	}
	defer cleanup()
	result, err := cli.Deploy(ctx, target.App, artifactPath, version)
	if err != nil {
		return err
	}
	if result.Skipped {
		fmt.Printf("already latest app=%s version=%s\n", target.App, result.Version)
		return nil
	}
	fmt.Printf("deployed app=%s version=%s\n", target.App, result.Version)
	printArtifactSummary(artifactPath)
	return nil
}

func gitDeploymentVersion(dir string) (string, error) {
	status := exec.Command("git", "status", "--porcelain")
	status.Dir = dir
	out, err := status.Output()
	if err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		return "", fmt.Errorf("git worktree is not clean; commit or remove local changes before deploy")
	}
	rev := exec.Command("git", "rev-parse", "--short=12", "HEAD")
	rev.Dir = dir
	version, err := rev.Output()
	if err != nil {
		return "", fmt.Errorf("git version failed: %w", err)
	}
	versionText := strings.TrimSpace(string(version))
	if versionText == "" {
		return "", fmt.Errorf("git version is empty")
	}
	return versionText, nil
}

func prepareDeployArtifact(artifactPath string, target project.Target) (string, func(), error) {
	cleanup := func() {}
	info, err := os.Stat(artifactPath)
	if err != nil {
		return "", cleanup, fmt.Errorf("artifact path invalid: %w", err)
	}
	if info.IsDir() {
		if err := prepareServiceArtifact(artifactPath, target); err != nil {
			return "", cleanup, err
		}
		return artifactPath, cleanup, nil
	}
	if strings.TrimSpace(target.Service.Command) == "" {
		return artifactPath, cleanup, nil
	}

	stagingDir, err := os.MkdirTemp("", "nova-artifact-stage-*")
	if err != nil {
		return "", cleanup, fmt.Errorf("create artifact staging dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(stagingDir) }
	stagedFile := filepath.Join(stagingDir, filepath.Base(artifactPath))
	if err := copyFile(artifactPath, stagedFile, info.Mode().Perm()); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := prepareServiceArtifact(stagingDir, target); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return stagingDir, cleanup, nil
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open artifact file: %w", err)
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create staged artifact file: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return fmt.Errorf("copy artifact file: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close staged artifact file: %w", err)
	}
	return nil
}

func prepareServiceArtifact(artifactDir string, target project.Target) error {
	if strings.TrimSpace(target.Service.Command) == "" {
		return nil
	}
	if err := writeRunScript(artifactDir, target.Service); err != nil {
		return err
	}
	files, err := topLevelArtifactFiles(artifactDir)
	if err != nil {
		return err
	}
	manifest := artifact.Manifest{
		App:      target.App,
		Artifact: artifact.ArtifactManifest{Files: files},
		Process:  artifact.ProcessManifest{Command: target.Service.Command},
		Runtime:  artifact.RuntimeManifest{HealthCommand: target.Service.HealthCommand},
	}
	if err := artifact.SaveManifest(artifactDir, manifest); err != nil {
		return err
	}
	return nil
}

func writeRunScript(artifactDir string, service project.ServiceConfig) error {
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}
	lines := []string{"#!/usr/bin/env sh", "set -eu"}
	keys := make([]string, 0, len(service.Env))
	for key := range service.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("export %s=%s", key, shellQuote(service.Env[key])))
	}
	lines = append(lines, "exec sh -lc "+shellQuote(service.Command))
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(artifactDir, "run"), []byte(content), 0o755); err != nil {
		return fmt.Errorf("write service run script: %w", err)
	}
	return nil
}

func topLevelArtifactFiles(artifactDir string) ([]string, error) {
	items, err := os.ReadDir(artifactDir)
	if err != nil {
		return nil, fmt.Errorf("read artifact dir: %w", err)
	}
	files := []string{}
	for _, item := range items {
		name := item.Name()
		if name == artifact.ManifestFile {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)
	return files, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func loadTargetFromArgs(args []string) (project.Target, error) {
	targets, err := loadTargetsFromArgs(args)
	if err != nil {
		return project.Target{}, err
	}
	if len(targets) != 1 {
		return project.Target{}, fmt.Errorf("expected exactly one configured app selector")
	}
	return targets[0], nil
}

func loadTargetsFromArgs(args []string) ([]project.Target, error) {
	if len(args) > 1 {
		return nil, fmt.Errorf("expected zero or one configured app selector")
	}
	selector := ""
	if len(args) == 1 {
		selector = args[0]
	}
	cfg, path, err := project.Load(".")
	if err != nil {
		return nil, err
	}
	fmt.Printf("project config: %s\n", path)
	if selector == "all" {
		targets, err := project.ResolveAll(cfg)
		if err != nil {
			return nil, err
		}
		fmt.Println("project app: all")
		return targets, nil
	}
	target, err := project.Resolve(cfg, selector)
	if err != nil {
		return nil, err
	}
	if target.Name != "" {
		fmt.Printf("project app: %s\n", target.Name)
	} else if selector != "" {
		fmt.Printf("project app: %s\n", selector)
	}
	return []project.Target{target}, nil
}

func loadLogTargetsFromArgs(args []string) ([]project.Target, bool, error) {
	follow := false
	selector := ""
	for _, arg := range args {
		if arg == "-f" {
			follow = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return nil, false, fmt.Errorf("unknown logs argument: %s", arg)
		}
		if selector != "" {
			return nil, false, fmt.Errorf("expected at most one configured app selector")
		}
		selector = arg
	}
	targets, err := loadTargetsFromArgs(optionalArg(selector))
	return targets, follow, err
}

func optionalArg(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}

func runShellCommands(ctx context.Context, commands []string) error {
	for _, command := range commands {
		fmt.Printf("$ %s\n", command)
		cmd := exec.CommandContext(ctx, "sh", "-lc", command)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
	}
	return nil
}

func targetLabel(target project.Target) string {
	if target.Name != "" {
		return target.Name
	}
	return target.App
}

func printArtifactSummary(artifactDir string) {
	manifest, ok, err := artifact.LoadManifest(artifactDir)
	if err != nil {
		fmt.Printf("artifact manifest skipped: %v\n", err)
		return
	}
	if !ok {
		return
	}
	summary := artifact.DeploymentSummary(manifest)
	if len(summary) == 0 {
		return
	}
	fmt.Println("artifact manifest:")
	for _, line := range summary {
		fmt.Printf("  %s\n", line)
	}
}

func ensureName(rest []string) {
	if len(rest) < 1 || strings.TrimSpace(rest[0]) == "" {
		fmt.Println("ERROR: missing app name")
		os.Exit(1)
	}
}

func runAgent(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	listen := fs.String("listen", ":32102", "Nova listen address")
	appRoot := fs.String("app-root", "/var/lib/nova/apps", "Artifact storage root")
	tokenFile := fs.String("token-file", "/etc/nova/token", "Token file")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("agent command only accepts flags")
	}

	if err := novaruntime.EnsureAppServiceTemplate(*appRoot); err != nil {
		return err
	}

	mux := agent.NewServer(*appRoot, readTokenFile(*tokenFile)).Handler()
	srv := &http.Server{
		Addr:    *listen,
		Handler: mux,
	}
	log.Printf("nova agent listening on %s (apps=%s)\n", *listen, *appRoot)
	return srv.ListenAndServe()
}

func readTokenFile(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func autoBootstrapRuntimeConfig(cmd string) error {
	if cmd == "agent" || cmd == "init" || cmd == "target" || isLifecycleCommand(cmd) {
		return nil
	}
	if runtimeConfigReady() {
		return nil
	}

	if !hasTTY() {
		return fmt.Errorf("未检测到发布目标配置。请先配置 NOVA_ENDPOINT（Nova Agent Endpoint）和 NOVA_TOKEN（访问令牌）环境变量")
	}

	info("未检测到发布目标配置，进入交互式初始化")
	return bootstrapRuntimeConfig()
}

func runTargetAdd(args []string) error {
	name := "default"
	var endpoint, token string
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "--url":
			if i+1 >= len(args) {
				return fmt.Errorf("--url requires a value")
			}
			i++
			endpoint = strings.TrimSpace(args[i])
		case "--token":
			if i+1 >= len(args) {
				return fmt.Errorf("--token requires a value")
			}
			i++
			token = strings.TrimSpace(args[i])
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unknown target add argument: %s", arg)
			}
			name = arg
		}
	}
	if name == "" {
		return fmt.Errorf("target name required")
	}
	if endpoint == "" {
		return fmt.Errorf("--url is required")
	}
	if token == "" {
		return fmt.Errorf("--token is required")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("读取用户目录失败: %w", err)
	}
	configPath := runtimeConfigPath(home)
	if err := writeConfig(configPath, endpoint, token); err != nil {
		return err
	}
	info(fmt.Sprintf("target %q 已写入 %s", name, configPath))
	return nil
}

func runTargetList() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("读取用户目录失败: %w", err)
	}
	configPath := runtimeConfigPath(home)
	endpoint, _, err := readRuntimeConfigFromFile(configPath)
	if err != nil {
		return fmt.Errorf("读取目标配置失败: %w", err)
	}
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("目标配置缺少 Nova Agent Endpoint")
	}
	fmt.Printf("* default %s\n", endpoint)
	return nil
}

func runtimeConfigReady() bool {
	endpoint := strings.TrimSpace(firstEnv("NOVA_ENDPOINT", "NOVA_AGENT_ENDPOINT"))
	token := strings.TrimSpace(firstEnv("NOVA_TOKEN", "NOVA_AGENT_TOKEN"))
	if endpoint != "" && token != "" {
		return true
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	endpointInFile, tokenInFile, err := readRuntimeConfigFromFile(runtimeConfigPath(home))
	if err != nil {
		return false
	}

	if endpoint == "" {
		endpoint = strings.TrimSpace(endpointInFile)
	}
	if token == "" {
		token = strings.TrimSpace(tokenInFile)
	}
	if endpoint == "" || token == "" {
		return false
	}

	_ = os.Setenv("NOVA_ENDPOINT", endpoint)
	_ = os.Setenv("NOVA_TOKEN", token)
	_ = os.Setenv("NOVA_AGENT_ENDPOINT", endpoint)
	_ = os.Setenv("NOVA_AGENT_TOKEN", token)
	return true
}

func bootstrapRuntimeConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("读取用户目录失败: %w", err)
	}

	configPath := runtimeConfigPath(home)

	endpoint := strings.TrimSpace(firstEnv("NOVA_ENDPOINT", "NOVA_AGENT_ENDPOINT"))
	if endpoint == "" {
		info("请填写 Nova Agent Endpoint，也就是已安装 Nova Agent 的机器地址，例如 http://服务器IP:32102")
		value, err := promptInput("Nova Agent Endpoint: ", "")
		if err != nil {
			return err
		}
		endpoint = value
	}
	if endpoint == "" {
		return fmt.Errorf("Nova Agent Endpoint 不能为空")
	}

	token := strings.TrimSpace(firstEnv("NOVA_TOKEN", "NOVA_AGENT_TOKEN"))
	if token == "" {
		value, err := promptInput("访问令牌（NOVA_TOKEN）: ", "")
		if err != nil {
			return err
		}
		token = value
	}
	if token == "" {
		return fmt.Errorf("访问令牌不能为空")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if _, isSet := firstEnvWithSource("NOVA_CLIENT_ENV"); !isSet {
			value, err := promptInput(fmt.Sprintf("配置文件路径 [%s]: ", configPath), configPath)
			if err != nil {
				return err
			}
			configPath = value
		}
	}

	if err := writeConfig(configPath, endpoint, token); err != nil {
		return err
	}
	info(fmt.Sprintf("配置已写入 %s", configPath))

	_ = os.Setenv("NOVA_ENDPOINT", endpoint)
	_ = os.Setenv("NOVA_TOKEN", token)
	_ = os.Setenv("NOVA_AGENT_ENDPOINT", endpoint)
	_ = os.Setenv("NOVA_AGENT_TOKEN", token)

	shellRC := detectShellRC(home)
	if shellRC != "" && hasTTY() {
		answer, err := promptInput(fmt.Sprintf("是否将配置自动加入 %s（y/N）? ", shellRC), "n")
		if err != nil {
			return err
		}
		if isYes(answer) {
			if err := appendSource(shellRC, configPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func readRuntimeConfigFromFile(path string) (string, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return parseRuntimeConfig(string(content))
}

func parseRuntimeConfig(content string) (string, string, error) {
	var endpoint, token string
	scan := bufio.NewScanner(bytes.NewBufferString(content))
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "export ") {
			continue
		}
		kv := strings.TrimSpace(strings.TrimPrefix(line, "export "))
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		switch key {
		case "NOVA_ENDPOINT", "NOVA_AGENT_ENDPOINT":
			if endpoint == "" {
				endpoint = val
			}
		case "NOVA_TOKEN", "NOVA_AGENT_TOKEN":
			if token == "" {
				token = val
			}
		}
	}
	if err := scan.Err(); err != nil {
		return "", "", err
	}
	return endpoint, token, nil
}

func defaultRuntimeConfigPath(home string) string {
	return filepath.Join(home, ".nova", "client.env")
}

func runtimeConfigPath(home string) string {
	return firstEnvOrDefault("NOVA_CLIENT_ENV", defaultRuntimeConfigPath(home))
}

type githubAsset struct {
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	Assets []githubAsset `json:"assets"`
}

func runInstall(_ []string) error {
	repo := strings.TrimSpace(os.Getenv("NOVA_GITHUB_REPO"))
	if repo == "" {
		repo = "luaxlou/nova-run"
	}

	info("Nova CLI 安装向导（单二进制模式）")
	info("执行此命令只负责安装/更新本地 CLI，不做服务器初始化")

	platformOS, platformArch, err := detectPlatform()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("读取用户目录失败: %w", err)
	}
	installDir := strings.TrimSpace(os.Getenv("NOVA_INSTALL_DIR"))
	clientBinaryName := firstEnvOrDefault("NOVA_BINARY_NAME", "nova")
	manualURL := strings.TrimSpace(firstEnv("NOVA_CLIENT_DOWNLOAD_URL", "NOVA_DOWNLOAD_URL"))

	binaryPath, useSudo, err := resolveInstallPath(installDir, home)
	if err != nil {
		return err
	}

	downloadURL := manualURL
	if downloadURL == "" {
		url, err := resolveDownloadURL(repo, clientBinaryName, platformOS, platformArch)
		if err != nil {
			return err
		}
		downloadURL = url
	}

	info(fmt.Sprintf("下载 CLI：%s", downloadURL))
	tmp, err := os.CreateTemp("", "nova-install-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	defer os.Remove(tmpPath)

	if err := downloadBinary(downloadURL, tmpPath); err != nil {
		return err
	}
	if isArchive(tmpPath) {
		return fmt.Errorf("下载结果为压缩包，当前不支持直接安装压缩包，请提供 NOVA_CLIENT_DOWNLOAD_URL 为直接二进制")
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("设置可执行权限失败: %w", err)
	}

	if err := os.MkdirAll(binaryPath, 0o755); err != nil {
		return fmt.Errorf("创建安装目录失败: %w", err)
	}
	target := filepath.Join(binaryPath, clientBinaryName)
	if useSudo {
		if err := moveWithSudo(tmpPath, target); err != nil {
			return err
		}
	} else {
		if err := os.Rename(tmpPath, target); err != nil {
			return fmt.Errorf("安装 CLI 失败: %w", err)
		}
	}
	if !isExecutable(target) {
		return fmt.Errorf("安装失败：未检测到可执行文件 %s", target)
	}
	info(fmt.Sprintf("CLI 已安装到：%s", target))
	return nil
}

func resolveInstallPath(installDir, home string) (string, bool, error) {
	if installDir != "" {
		return installDir, false, nil
	}

	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Local", "Nova", "bin"), false, nil
	}

	systemDir := "/usr/local/bin"
	if canWriteDir(systemDir) {
		return systemDir, false, nil
	}
	if hasCommand("sudo") {
		return systemDir, true, nil
	}
	return filepath.Join(home, ".local", "bin"), false, nil
}

func detectPlatform() (string, string, error) {
	osName := runtime.GOOS
	switch osName {
	case "linux":
	case "darwin":
	case "windows":
	default:
		return "", "", fmt.Errorf("不支持当前运行平台: %s", osName)
	}

	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "amd64"
	case "386":
		arch = "386"
	case "arm64":
		arch = "arm64"
	case "arm":
		arch = "arm"
	default:
		return "", "", fmt.Errorf("不支持当前 CPU 架构: %s", arch)
	}
	return osName, arch, nil
}

func resolveDownloadURL(repo, binary, osName, arch string) (string, error) {
	base := "https://github.com/" + repo
	candidates := []string{
		fmt.Sprintf("%s/releases/latest/download/%s-%s-%s", base, binary, osName, arch),
		fmt.Sprintf("%s/releases/latest/download/%s-%s-%s.tar.gz", base, binary, osName, arch),
		fmt.Sprintf("%s/releases/latest/download/%s-%s-%s.zip", base, binary, osName, arch),
	}

	for _, u := range candidates {
		ok, err := urlExists(u)
		if err != nil {
			continue
		}
		if ok {
			return u, nil
		}
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "nova-install")
	resp, err := newHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 GitHub 发布失败: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("请求 GitHub 发布失败: %s", resp.Status)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 GitHub 发布信息失败: %w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(payload, &release); err != nil {
		return "", fmt.Errorf("解析 GitHub 发布信息失败: %w", err)
	}

	pattern := fmt.Sprintf("%s-%s-%s", binary, osName, arch)
	for _, asset := range release.Assets {
		url := asset.BrowserDownloadURL
		if strings.HasSuffix(url, "/"+pattern) || strings.HasSuffix(url, "/"+pattern+".tar.gz") || strings.HasSuffix(url, "/"+pattern+".zip") {
			return url, nil
		}
		if strings.Contains(url, "/"+pattern+".") {
			return url, nil
		}
	}
	return "", fmt.Errorf("未找到匹配平台的可下载二进制 (%s, %s, %s)", repo, osName, arch)
}

func downloadBinary(url, dst string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败: %w", err)
	}
	resp, err := newHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载失败: %s", resp.Status)
	}
	file, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	return nil
}

func isArchive(path string) bool {
	fp, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() {
		_ = fp.Close()
	}()
	header := make([]byte, 4)
	n, _ := io.ReadAtLeast(fp, header, 2)
	if n < 2 {
		return false
	}
	if header[0] == 0x1f && header[1] == 0x8b {
		return true
	}
	if n >= 4 && header[0] == 0x50 && header[1] == 0x4b && (header[2] == 0x03 || header[2] == 0x05 || header[2] == 0x07) && (header[3] == 0x04 || header[3] == 0x06 || header[3] == 0x08) {
		return true
	}
	return false
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode()&0o111 != 0
}

func writeConfig(path, endpoint, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	content := fmt.Sprintf("export NOVA_ENDPOINT=%q\nexport NOVA_TOKEN=%q\nexport NOVA_AGENT_ENDPOINT=%q\nexport NOVA_AGENT_TOKEN=%q\n", endpoint, token, endpoint, token)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	return nil
}

func appendSource(shellRC, configPath string) error {
	line := fmt.Sprintf("source \"%s\"", configPath)
	content, err := os.ReadFile(shellRC)
	if err == nil && strings.Contains(string(content), line) {
		info(fmt.Sprintf("已检测到配置文件已在 %s 中。", shellRC))
		return nil
	}

	f, err := os.OpenFile(shellRC, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("打开 shell 配置文件失败: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.WriteString("\n# Nova runtime env\n"); err != nil {
		return fmt.Errorf("写入 shell 配置失败: %w", err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("写入 shell 配置失败: %w", err)
	}
	info(fmt.Sprintf("已写入：%s 到 %s", line, shellRC))
	return nil
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func canWriteDir(dir string) bool {
	fp, err := os.OpenFile(filepath.Join(dir, ".nova-install-tmp"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	_ = fp.Close()
	_ = os.Remove(fp.Name())
	return true
}

func moveWithSudo(src, dst string) error {
	cmd := exec.Command("sudo", "mv", src, dst)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("安装文件失败: %w", err)
	}
	return nil
}

func promptInput(message, defaultValue string) (string, error) {
	reader := os.Stdin
	if tty, err := os.Open("/dev/tty"); err == nil {
		defer func() {
			_ = tty.Close()
		}()
		reader = tty
	}
	fmt.Print(message)
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = defaultValue
	}
	return line, nil
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func firstEnvWithSource(key string) (string, bool) {
	v := strings.TrimSpace(os.Getenv(key))
	return v, v != ""
}

func firstEnvOrDefault(key, value string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return value
}

func isYes(value string) bool {
	return strings.EqualFold(value, "y") || strings.EqualFold(value, "yes")
}

func isHelp(value string) bool {
	return value == "-h" || value == "--help" || strings.EqualFold(value, "help")
}

func hasTTY() bool {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	_ = tty.Close()
	return true
}

func detectShellRC(home string) string {
	switch filepath.Base(os.Getenv("SHELL")) {
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "bash":
		return filepath.Join(home, ".bashrc")
	default:
		if runtime.GOOS == "windows" {
			return ""
		}
		return filepath.Join(home, ".profile")
	}
}

func urlExists(url string) (bool, error) {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := newHTTPClient().Do(req)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return resp.StatusCode >= 200 && resp.StatusCode < 400, nil
}

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second, Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 15 * time.Second,
		}).DialContext,
		DisableKeepAlives: false,
	}}
}

func info(message string) {
	fmt.Printf("[nova] %s\n", message)
}
