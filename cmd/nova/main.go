package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	"strings"
	"time"

	"github.com/luaxlou/glow-ops/internal/agent"
	"github.com/luaxlou/glow-ops/internal/client"
)

func usage() {
	fmt.Println(`nova (Nova Run CLI)

Usage:
  nova                # 无参数时会检查本地配置，不存在则进入交互式初始化
  nova install        # 安装/更新本地 Nova CLI 二进制
  nova agent --listen :32102 --app-root /var/lib/nova/apps --token-file /etc/nova/token
  nova deploy <app> <artifact_dir>
  nova start <app>
  nova stop <app>
  nova restart <app>
  nova status <app>
  nova logs <app> [-f]
  nova list
  nova remove <app>

Local convenience:
  nova rollback <app>
  nova target add <name> --url <url> --token <token>
  nova target use <name>
  nova target list
`)
}

func main() {
	args := os.Args
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

	ctx := context.Background()
	cli := client.NewClient()
	cmd := args[1]
	rest := args[2:]

	switch cmd {
	case "install":
		if err := runInstall(rest); err != nil {
			fmt.Printf("install failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("install finished")
	case "agent":
		if err := runAgent(rest); err != nil {
			fmt.Printf("agent failed: %v\n", err)
			os.Exit(1)
		}
	case "deploy":
		if len(rest) < 2 {
			fmt.Println("ERROR: nova deploy <app> <artifact_dir>")
			os.Exit(1)
		}
		if err := cli.Deploy(ctx, rest[0], rest[1]); err != nil {
			fmt.Printf("deploy failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("deployed")
	case "start":
		ensureName(rest)
		if err := cli.Start(ctx, rest[0]); err != nil {
			fmt.Printf("start failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("start command sent")
	case "stop":
		ensureName(rest)
		if err := cli.Stop(ctx, rest[0]); err != nil {
			fmt.Printf("stop failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("stop command sent")
	case "restart":
		ensureName(rest)
		if err := cli.Restart(ctx, rest[0]); err != nil {
			fmt.Printf("restart failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("restart command sent")
	case "status":
		ensureName(rest)
		status, err := cli.Status(ctx, rest[0])
		if err != nil {
			fmt.Printf("status failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(status)
	case "logs":
		ensureName(rest)
		follow := len(rest) > 1 && strings.EqualFold(rest[1], "-f")
		if follow {
			if err := cli.LogsStream(ctx, rest[0], os.Stdout); err != nil {
				fmt.Printf("logs failed: %v\n", err)
				os.Exit(1)
			}
		} else {
			stream, err := cli.Logs(ctx, rest[0], follow)
			if err != nil {
				fmt.Printf("logs failed: %v\n", err)
				os.Exit(1)
			}
			for _, line := range stream {
				fmt.Println(line)
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
		ensureName(rest)
		if err := cli.Remove(ctx, rest[0]); err != nil {
			fmt.Printf("remove failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("removed")
	case "rollback":
		ensureName(rest)
		fmt.Println("rollback is a local operation; not implemented in this milestone")
	case "target":
		if len(rest) == 0 {
			usage()
			os.Exit(1)
		}
		switch rest[0] {
		case "add":
			fmt.Println("target add: use nova config file at ~/.nova/contexts (not implemented in this milestone)")
		case "use":
			fmt.Println("target use: use nova config file at ~/.nova/contexts (not implemented in this milestone)")
		case "list":
			fmt.Println("target list: use nova config file at ~/.nova/contexts (not implemented in this milestone)")
		default:
			usage()
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
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
	if cmd == "agent" || cmd == "install" {
		return nil
	}
	if runtimeConfigReady() {
		return nil
	}

	if !hasTTY() {
		return fmt.Errorf("未检测到 Nova 运行时配置。请先配置 NOVA_ENDPOINT 和 NOVA_TOKEN 环境变量")
	}

	info("未检测到 Nova 运行时配置，进入交互式初始化")
	return bootstrapRuntimeConfig()
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
	endpointInFile, tokenInFile, err := readRuntimeConfigFromFile(defaultRuntimeConfigPath(home))
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

	configPath := firstEnvOrDefault("NOVA_CLIENT_ENV", defaultRuntimeConfigPath(home))
	if v, isSet := firstEnvWithSource("NOVA_CLIENT_ENV"); isSet {
		configPath = v
	}

	endpoint := strings.TrimSpace(firstEnv("NOVA_ENDPOINT", "NOVA_AGENT_ENDPOINT"))
	if endpoint == "" {
		value, err := promptInput("Nova 地址 [http://127.0.0.1:32102]: ", "http://127.0.0.1:32102")
		if err != nil {
			return err
		}
		endpoint = value
	}

	token := strings.TrimSpace(firstEnv("NOVA_TOKEN", "NOVA_AGENT_TOKEN"))
	if token == "" {
		value, err := promptInput("Nova Token: ", "")
		if err != nil {
			return err
		}
		token = value
	}
	if token == "" {
		return fmt.Errorf("Token 不能为空")
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
	if shellRC != "" {
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
