package manager

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/luaxlou/glow-ops/internal/configmanager"
	"github.com/luaxlou/glow-ops/internal/statemanager"
	"github.com/luaxlou/glow-ops/pkg/api"
)

var (
	mu sync.RWMutex
)

type StopAppOptions struct {
	KeepIngress bool
}

func StartApp(req api.StartAppRequest) error {
	mu.Lock()
	defer mu.Unlock()

	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}
	if absDir, err := filepath.Abs(dataDir); err == nil {
		dataDir = absDir
	}

	var existingRestartCount int
	var existingDomain string
	var existingConfigHash string
	var existingBinaryHash string
	var existingApp *api.AppInfo

	log.Printf("StartApp: %s (Command: %s)", req.Name, req.Command)

	if app, err := statemanager.GetApp(req.Name); err == nil {
		existingApp = app
		log.Printf("Found existing app: %s (Status: %s, Command: %s)", app.Name, app.Status, app.Command)
		if existingApp.Status == "RUNNING" {
			// Idempotent: return success if already running
			return nil
		}
		if existingApp.Status != "STOPPED" {
			existingRestartCount = existingApp.RestartCount
		}
		existingDomain = existingApp.Domain
		existingConfigHash = existingApp.ConfigHash
		existingBinaryHash = existingApp.BinaryHash
	}

	// Merge existing app info if request fields are missing
	if existingApp != nil {
		// Defer Command inheritance until after checking deployed binary
		if req.Args == nil {
			req.Args = existingApp.Args
		}
		if req.WorkingDir == "" {
			req.WorkingDir = existingApp.WorkingDir
		}
		if req.Env == nil {
			req.Env = existingApp.Env
		}
		// Inherit AutoRestart if Command is missing (likely a restart)
		if req.Command == "" {
			req.AutoRestart = existingApp.AutoRestart
		}
	}

	// Port semantics:
	// - If an app explicitly declares an open port (via apply / metadata), it will be stored in DB and reused here.
	// - If no port is declared, the app is treated as "not exposing a port" and the server MUST NOT auto-allocate one.
	port := 0
	if existingApp != nil && existingApp.Port != 0 {
		port = existingApp.Port
	}

	// 1. Prepare Directory: apps/<name>
	appDir := filepath.Join(dataDir, "apps", req.Name)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return fmt.Errorf("failed to create app dir: %w", err)
	}

	// 2. Rename Binary: nova_<name>
	srcBinary := req.Command

	dstBinaryName := "nova_" + req.Name
	dstBinaryPath := filepath.Join(appDir, dstBinaryName)

	// If srcBinary is empty, try to fall back to the previously deployed binary.
	// Priority: 1) deployed nova_<name>, 2) existingApp.Command, 3) error
	if srcBinary == "" {
		if _, err := os.Stat(dstBinaryPath); err == nil {
			// Use the deployed binary
			srcBinary = dstBinaryPath
			req.Command = dstBinaryPath
		} else if existingApp != nil && existingApp.Command != "" {
			// Fall back to existing command if it exists
			if _, err := os.Stat(existingApp.Command); err == nil {
				srcBinary = existingApp.Command
				req.Command = existingApp.Command
			} else {
				return fmt.Errorf("app '%s' not found or no command specified", req.Name)
			}
		} else {
			return fmt.Errorf("app '%s' not found or no command specified", req.Name)
		}
	}

	// Check if src and dst are the same file to avoid overwriting
	isSameFile := false
	if absSrc, err := filepath.Abs(srcBinary); err == nil {
		if absDst, err := filepath.Abs(dstBinaryPath); err == nil {
			if absSrc == absDst {
				isSameFile = true
			}
		}
	}

	// Attempt to copy if source exists and is not the same as destination
	if !isSameFile {
		if _, err := os.Stat(srcBinary); err == nil {
			if err := copyFile(srcBinary, dstBinaryPath); err != nil {
				return fmt.Errorf("failed to copy binary: %w", err)
			}
			if err := os.Chmod(dstBinaryPath, 0755); err != nil {
				return fmt.Errorf("failed to chmod binary: %w", err)
			}
		} else {
			// If source not found, check if destination already exists (restart scenario)
			if _, err := os.Stat(dstBinaryPath); err != nil {
				// If neither exists, fallback if it looks like a command
				if strings.Contains(req.Command, string(os.PathSeparator)) {
					return fmt.Errorf("binary not found: %s", req.Command)
				}
				dstBinaryPath = req.Command
			}
		}
	} else {
		// If same file, verify it exists
		if _, err := os.Stat(dstBinaryPath); err != nil {
			return fmt.Errorf("binary not found at expected location: %s", dstBinaryPath)
		}
	}

	// Calculate Binary Hash
	currentBinaryHash, err := calculateFileHash(dstBinaryPath)
	if err == nil {
		existingBinaryHash = currentBinaryHash
	}

	app := api.AppInfo{
		Name:         req.Name,
		Command:      dstBinaryPath,
		Args:         req.Args,
		WorkingDir:   req.WorkingDir,
		Env:          req.Env,
		Port:         port,
		Domain:       existingDomain,
		// AutoRestart removed: server no longer keeps apps alive automatically.
		AutoRestart:  false,
		RestartCount: existingRestartCount,
		Status:       "STARTING",
		ConfigHash:   existingConfigHash,
		BinaryHash:   existingBinaryHash,
	}

	if app.WorkingDir == "" {
		app.WorkingDir = appDir
	}

	if !req.SkipIngress && app.Port != 0 && app.Domain != "" {
		if err := GenerateNginxConfig(dataDir, NginxConfig{
			Name:   app.Name,
			Port:   app.Port,
			Domain: app.Domain,
		}); err != nil {
			fmt.Printf("Warning: Failed to generate nginx config: %v\n", err)
		}
	}

	cmd := exec.Command(app.Command, app.Args...)
	cmd.Dir = app.WorkingDir

	cmd.Env = os.Environ()
	for k, v := range app.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	if port != 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("OP_APP_PORT=%d", port))
	}
	if app.Domain != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("OP_APP_DOMAIN=%s", app.Domain))
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("OP_APP_NAME=%s", app.Name))

	// 3. Run as nova user
	if u, err := user.Lookup("nova"); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
		}
	} else {
		// Fallback: If 'nova' user doesn't exist, try to use current user
		// But for debugging exec format error, we might want to ensure we aren't switching to a user with different env
		// log.Printf("Warning: 'nova' user not found, running as current user")
	}

	// 4. Logs with rotation
	logDir := filepath.Join(appDir, "logs")
	os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, app.Name+".log")

	rotator, err := NewLogRotator(logFile, 10*1024*1024, 5)
	if err == nil {
		cmd.Stdout = rotator
		cmd.Stderr = rotator
	} else {
		f, _ := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		cmd.Stdout = f
		cmd.Stderr = f
	}

	if err := cmd.Start(); err != nil {
		app.Status = "ERROR"
		statemanager.SaveApp(app)
		return fmt.Errorf("failed to start app: %w", err)
	}

	app.Status = "RUNNING"
	app.Pid = cmd.Process.Pid
	app.StartTime = time.Now().UnixMilli()
	statemanager.SaveApp(app)

	go waitForExit(app.Name, cmd)

	return nil
}

func waitForExit(name string, cmd *exec.Cmd) {
	err := cmd.Wait()

	mu.Lock()
	defer mu.Unlock()

	app, errGet := statemanager.GetApp(name)
	if errGet != nil {
		return
	}

	// Check if we are still the active process
	if app.Pid != cmd.Process.Pid {
		return
	}

	// If manual stop, status is already STOPPED
	if app.Status == "STOPPED" {
		app.Pid = 0
		statemanager.SaveApp(*app)
		return
	}

	if err != nil {
		app.Status = "ERROR"
	} else {
		// Normal exit is also treated as ERROR for auto-restart purposes unless manual stop
		// Spec says: "WHEN 应用进程退出 (无论 Exit Code 为何) 且状态非 STOPPED -> THEN 系统应更新状态为 ERROR"
		app.Status = "ERROR"
	}
	app.Pid = 0
	statemanager.SaveApp(*app)
}

// ListResources returns a summary of all managed resources (Apps, etc.)
func ListResources() ([]api.ResourceRef, error) {
	var resources []api.ResourceRef

	// 1. Apps
	apps, err := statemanager.ListApps()
	if err == nil {
		for _, app := range apps {
			resources = append(resources, api.ResourceRef{
				Kind: "App",
				Name: app.Name,
				Port: app.Port,
			})
		}
	} else {
		log.Printf("Warning: Failed to list apps for resource summary: %v", err)
	}

	return resources, nil
}

func StopApp(name string) error {
	return StopAppWithOptions(name, StopAppOptions{})
}

func StopAppWithOptions(name string, opts StopAppOptions) error {
	mu.Lock()
	defer mu.Unlock()

	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}

	app, err := statemanager.GetApp(name)
	if err != nil {
		return fmt.Errorf("app %s not found", name)
	}

	if app.Pid == 0 {
		return nil
	}

	process, err := os.FindProcess(app.Pid)
	if err != nil {
		return err
	}

	err = process.Signal(syscall.SIGTERM)
	if err != nil {
		process.Kill()
	}

	app.Status = "STOPPED"
	app.Pid = 0
	if !opts.KeepIngress {
		RemoveNginxConfig(dataDir, name)
	}
	return statemanager.SaveApp(*app)
}

func DeleteApp(name string) error {
	mu.Lock()
	defer mu.Unlock()

	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}

	app, err := statemanager.GetApp(name)
	if err != nil {
		// If app doesn't exist in DB, check filesystem just in case
		appDir := filepath.Join(dataDir, "apps", name)
		if _, err := os.Stat(appDir); !os.IsNotExist(err) {
			os.RemoveAll(appDir)
		}
		return nil
	}

	// 1. Stop if running
	if app.Pid > 0 {
		if proc, err := os.FindProcess(app.Pid); err == nil {
			proc.Signal(syscall.SIGTERM)
			// Give it a moment, then force kill if needed?
			// For now just fire signal
		}
	}

	// 2. Remove Nginx Config
	RemoveNginxConfig(dataDir, name)

	// 3. Remove App Directory (logs, binaries)
	appDir := filepath.Join(dataDir, "apps", name)
	os.RemoveAll(appDir)

	// 4. Remove from State
	return statemanager.DeleteApp(name)
}

func ListApps() []api.AppInfo {
	dbApps, err := statemanager.ListApps()
	if err != nil {
		return []api.AppInfo{}
	}
	return dbApps
}

func copyFile(src, dst string) error {
	if src == dst {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func calculateFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
