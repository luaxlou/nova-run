package apiserver

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luaxlou/glow-ops/internal/configmanager"
	"github.com/luaxlou/glow-ops/internal/manager"
	"github.com/luaxlou/glow-ops/internal/provisioner"
	"github.com/luaxlou/glow-ops/internal/statemanager"
	"github.com/luaxlou/glow-ops/pkg/api"
	"github.com/shirou/gopsutil/v3/process"
)

type Server struct{}

func New() *Server {
	return &Server{}
}

func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", s.handleHealth)

	// All management APIs require CLI authentication (Bearer API Key).
	protected := r.Group("/", RequireAPIKey())

	// --- Config Management ---
	protected.GET("/config/:appName", s.handleGetConfig)
	protected.PUT("/config/:appName", s.handleUpdateConfig)
	protected.POST("/config/:appName/render", s.handleRenderConfig) // New: render config to disk

	// --- App Management ---
	protected.POST("/apps/upload", s.handleUploadApp)
	protected.PUT("/apps/:name", s.handleUpsertApp) // New: upsert app metadata (apply)
	protected.POST("/apps/start", s.handleStartApp)
	protected.POST("/apps/stop", s.handleStopApp)
	protected.POST("/apps/restart", s.handleRestartApp)
	protected.POST("/apps/delete", s.handleDeleteApp)
	protected.GET("/apps/list", s.handleListApps)
	protected.GET("/apps/:name", s.handleGetApp) // New: get single app details
	protected.GET("/apps/logs", s.handleAppLogs)

	// --- Resource Binding ---
	protected.POST("/apps/:appName/resources/mysql", s.handleBindMySQL) // New: bind MySQL resource
	protected.POST("/apps/:appName/resources/redis", s.handleBindRedis) // New: bind Redis resource

	// --- Node Management ---
	protected.GET("/node/status", s.handleNodeStatus) // New

	// --- Server Management ---
	protected.GET("/server/info", s.handleServerInfo) // New

	// --- Ingress Management ---
	protected.POST("/ingress/update", s.handleUpdateIngress)
	protected.POST("/ingress/delete", s.handleDeleteIngress)
	protected.GET("/ingress/list", s.handleListIngress)
}

func (s *Server) handleHealth(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

func (s *Server) handleUpdateIngress(c *gin.Context) {
	var req api.IngressUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "Invalid request body"})
		return
	}

	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}

	port := req.Port
	if port == 0 {
		// Try to find app port
		apps := manager.ListApps()
		for _, app := range apps {
			if app.Name == req.AppName {
				port = app.Port
				break
			}
		}
	}

	if port == 0 {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "App not found or port not specified"})
		return
	}

	if err := manager.GenerateNginxConfig(dataDir, manager.NginxConfig{
		Name:   req.AppName,
		Port:   port,
		Domain: req.Domain,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: err.Error()})
		return
	}

	// Update AppInfo in StateManager
	if app, err := statemanager.GetApp(req.AppName); err == nil {
		app.Domain = req.Domain
		statemanager.SaveApp(*app)
	}

	c.JSON(http.StatusOK, api.Response{Success: true, Message: "Ingress updated"})
}

type upsertAppRequest struct {
	Name       string            `json:"name"`
	Command    string            `json:"command"`
	Port       int               `json:"port"`
	Args       []string          `json:"args"`
	WorkingDir string            `json:"workingDir"`
	Domain     string            `json:"domain"`
	Env        map[string]string `json:"env"`
	Config     map[string]any    `json:"config"`
}

func (s *Server) handleUpsertApp(c *gin.Context) {
	name := c.Param("name")
	var req upsertAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "Invalid request body"})
		return
	}
	if req.Name != "" && req.Name != name {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "name mismatch"})
		return
	}

	// Port rule: if port is not specified in apply, it should be treated as "not exposed" (port=0).
	// Domain requires an exposed port.
	if req.Domain != "" && req.Port == 0 {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "domain requires a non-zero port"})
		return
	}
	if req.Args == nil {
		req.Args = []string{}
	}
	if req.Env == nil {
		req.Env = map[string]string{}
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}

	app, err := statemanager.GetApp(name)
	if err != nil || app == nil {
		app = &api.AppInfo{Name: name}
	}

	// Get dataDir for default paths
	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}
	if absDir, err := filepath.Abs(dataDir); err == nil {
		dataDir = absDir
	}
	appDir := filepath.Join(dataDir, "apps", name)

	// Convention over configuration: set defaults if not specified
	if req.Command == "" {
		// Default to <app-name> binary in app directory
		req.Command = filepath.Join(appDir, name)
	}
	if req.WorkingDir == "" {
		// Default to app directory
		req.WorkingDir = appDir
	}

	// Overwrite declarative fields from apply.
	app.Name = name
	app.Command = req.Command
	app.Port = req.Port
	app.Args = req.Args
	app.Domain = req.Domain
	app.WorkingDir = req.WorkingDir
	app.Env = req.Env
	app.Config = req.Config

	if err := statemanager.SaveApp(*app); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: err.Error()})
		return
	}
	if err := configmanager.Set(name, req.Config, false); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, api.Response{Success: true, Message: "App updated", Data: app})
}

func (s *Server) handleDeleteIngress(c *gin.Context) {
	var req api.IngressDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "Invalid request body"})
		return
	}

	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}

	if err := manager.RemoveNginxConfig(dataDir, req.AppName); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: err.Error()})
		return
	}

	// Update AppInfo in StateManager
	if app, err := statemanager.GetApp(req.AppName); err == nil {
		app.Domain = ""
		statemanager.SaveApp(*app)
	}

	c.JSON(http.StatusOK, api.Response{Success: true, Message: "Ingress deleted"})
}

func (s *Server) handleListIngress(c *gin.Context) {
	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}

	configs, err := manager.ListIngress(dataDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, api.Response{Success: true, Data: configs})
}

func (s *Server) handleGetConfig(c *gin.Context) {
	appName := c.Param("appName")
	app, err := statemanager.GetApp(appName)
	if err != nil {
		c.JSON(http.StatusNotFound, api.Response{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, api.Response{Success: true, Data: app.Config})
}

func (s *Server) handleUpdateConfig(c *gin.Context) {
	appName := c.Param("appName")
	var newConfig map[string]any
	if err := c.ShouldBindJSON(&newConfig); err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "Invalid JSON"})
		return
	}

	app, err := statemanager.GetApp(appName)
	if err != nil {
		c.JSON(http.StatusNotFound, api.Response{Success: false, Message: "App not found"})
		return
	}

	merge := c.Query("merge") != "false"
	if merge {
		if app.Config == nil {
			app.Config = make(map[string]any)
		}
		for k, v := range newConfig {
			app.Config[k] = v
		}
	} else {
		app.Config = newConfig
	}

	if err := statemanager.SaveApp(*app); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: "Failed to update config"})
		return
	}
	if err := configmanager.Set(appName, app.Config, false); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, api.Response{Success: true, Message: "Config updated"})
}

func (s *Server) handleRenderConfig(c *gin.Context) {
	appName := c.Param("appName")

	// Get the config from statemanager
	app, err := statemanager.GetApp(appName)
	if err != nil {
		c.JSON(http.StatusNotFound, api.Response{Success: false, Message: err.Error()})
		return
	}
	config := app.Config
	if config == nil {
		config = make(map[string]any)
	}

	// Get dataDir from system config
	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}
	if absDir, err := filepath.Abs(dataDir); err == nil {
		dataDir = absDir
	}

	// Ensure app directory exists
	appDir := filepath.Join(dataDir, "apps", appName)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: fmt.Sprintf("Failed to create app directory: %v", err)})
		return
	}

	// Write config to disk
	configFileName := "config.json"
	configFilePath := filepath.Join(appDir, configFileName)
	configBytes, err := json.Marshal(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: fmt.Sprintf("Failed to marshal config: %v", err)})
		return
	}

	if err := os.WriteFile(configFilePath, configBytes, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: fmt.Sprintf("Failed to write config file: %v", err)})
		return
	}

	// Calculate simple hash for verification
	configHash := fmt.Sprintf("%x", md5.Sum(configBytes))

	// Update ConfigHash in AppInfo (Save back to statemanager)
	app.ConfigHash = configHash
	statemanager.SaveApp(*app)

	c.JSON(http.StatusOK, api.Response{
		Success: true,
		Message: "Config rendered successfully",
		Data: map[string]any{
			"path":       configFilePath,
			"bytes":      len(configBytes),
			"configHash": configHash,
		},
	})
}

func (s *Server) handleUploadApp(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "File required"})
		return
	}

	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}
	if absDir, err := filepath.Abs(dataDir); err == nil {
		dataDir = absDir
	}
	tempDir := filepath.Join(dataDir, "tmp", "uploads")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: "Failed to create temp dir"})
		return
	}

	// Use original filename but safe
	dst := filepath.Join(tempDir, filepath.Base(file.Filename))
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: "Failed to save file"})
		return
	}

	c.JSON(http.StatusOK, api.Response{Success: true, Data: dst})
}

func (s *Server) handleStartApp(c *gin.Context) {
	var req api.StartAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "Invalid request body"})
		return
	}
	if err := manager.StartApp(req); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, api.Response{Success: true, Message: "App started"})
}

func (s *Server) handleStopApp(c *gin.Context) {
	var req api.StopAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "Invalid request body"})
		return
	}
	if err := manager.StopAppWithOptions(req.Name, manager.StopAppOptions{KeepIngress: req.KeepIngress}); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, api.Response{Success: true, Message: "App stopped"})
}

func (s *Server) handleDeleteApp(c *gin.Context) {
	var req api.StopAppRequest // Re-use StopAppRequest as it just needs Name
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "Invalid request body"})
		return
	}
	if err := manager.DeleteApp(req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, api.Response{Success: true, Message: "App deleted"})
}

func (s *Server) handleRestartApp(c *gin.Context) {
	var req api.StopAppRequest // Use StopAppRequest struct as it just needs Name
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "Invalid request body"})
		return
	}

	// 1. Find existing app info to preserve config/env/args
	apps := manager.ListApps()
	var targetApp *api.AppInfo
	for _, app := range apps {
		if app.Name == req.Name {
			targetApp = &app
			break
		}
	}

	if targetApp == nil {
		c.JSON(http.StatusNotFound, api.Response{Success: false, Message: "App not found"})
		return
	}

	// Legacy support: apps may have registered without Command/WorkingDir. Try to infer from PID.
	if targetApp.Command == "" && targetApp.Pid != 0 {
		if p, err := process.NewProcess(int32(targetApp.Pid)); err == nil {
			if exe, err := p.Exe(); err == nil && exe != "" {
				targetApp.Command = exe
			}
			if targetApp.WorkingDir == "" {
				if cwd, err := p.Cwd(); err == nil && cwd != "" {
					targetApp.WorkingDir = cwd
				}
			}
		}
	}

	// If Command is still empty, check for deployed binary
	if targetApp.Command == "" {
		dataDir, _ := configmanager.GetSystemConfig("data_dir")
		if dataDir == "" {
			dataDir = "."
		}
		if absDir, err := filepath.Abs(dataDir); err == nil {
			dataDir = absDir
		}
		appDir := filepath.Join(dataDir, "apps", targetApp.Name)
		dstBinaryPath := filepath.Join(appDir, "nova_"+targetApp.Name)

		// Check if deployed binary exists
		if _, err := os.Stat(dstBinaryPath); err == nil {
			targetApp.Command = dstBinaryPath
		} else {
			c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: fmt.Sprintf("Failed to restart: app '%s' not found or no command specified", req.Name)})
			return
		}
	}

	// 2. Stop first (ignore error if already stopped)
	manager.StopApp(req.Name)

	// 3. Start again with same parameters
	startReq := api.StartAppRequest{
		Name:        targetApp.Name,
		Command:     targetApp.Command,
		Args:        targetApp.Args,
		WorkingDir:  targetApp.WorkingDir,
		Env:         targetApp.Env,
		AutoRestart: targetApp.AutoRestart,
		Config:      targetApp.Config,
	}

	if err := manager.StartApp(startReq); err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: fmt.Sprintf("Failed to restart: %v", err)})
		return
	}

	c.JSON(http.StatusOK, api.Response{Success: true, Message: "App restarted"})
}

func (s *Server) handleListApps(c *gin.Context) {
	apps := manager.ListApps()

	// Dynamically query process info for each app
	for i := range apps {
		if apps[i].Pid != 0 {
			if p, err := process.NewProcess(int32(apps[i].Pid)); err == nil {
				// Process exists - update status and metrics
				apps[i].Status = "RUNNING"
				if cpuPercent, err := p.CPUPercent(); err == nil {
					apps[i].Stats.CPUPercent = cpuPercent
				}
				if memInfo, err := p.MemoryInfo(); err == nil {
					apps[i].Stats.MemoryUsage = uint64(memInfo.RSS)
				}
				if createTime, err := p.CreateTime(); err == nil {
					apps[i].StartTime = createTime / 1000
				}
			} else {
				// Process doesn't exist
				if apps[i].Status != "STOPPED" {
					apps[i].Status = "EXITED"
					apps[i].Pid = 0
				}
			}
		}
	}

	c.JSON(http.StatusOK, api.Response{Success: true, Data: apps})
}

func (s *Server) handleAppLogs(c *gin.Context) {
	appName := c.Query("name")
	if appName == "" {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "name required"})
		return
	}

	// Get dataDir from system config
	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}

	// Simple log reading
	logFile := filepath.Join(dataDir, "apps", appName, "logs", appName+".log")
	if _, err := os.Stat(logFile); err != nil {
		c.JSON(http.StatusNotFound, api.Response{Success: false, Message: "Log file not found"})
		return
	}

	content, err := os.ReadFile(logFile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: "Failed to read logs"})
		return
	}

	// Limit log size? For now return all.
	c.JSON(http.StatusOK, api.Response{Success: true, Data: string(content)})
}

func (s *Server) handleNodeStatus(c *gin.Context) {
	node, err := manager.GetNodeStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.Response{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, api.Response{Success: true, Data: node})
}

func (s *Server) handleGetApp(c *gin.Context) {
	appName := c.Param("name")

	apps := manager.ListApps()
	var targetApp *api.AppInfo
	for i := range apps {
		if apps[i].Name == appName {
			targetApp = &apps[i]
			break
		}
	}

	if targetApp == nil {
		c.JSON(http.StatusNotFound, api.Response{Success: false, Message: "App not found"})
		return
	}

	// Dynamically query process info if PID exists
	if targetApp.Pid != 0 {
		if p, err := process.NewProcess(int32(targetApp.Pid)); err == nil {
			// Update CPU, Memory, IO info
			targetApp.Status = "RUNNING"
			if cpuPercent, err := p.CPUPercent(); err == nil {
				targetApp.Stats.CPUPercent = cpuPercent
			}
			if memInfo, err := p.MemoryInfo(); err == nil {
				targetApp.Stats.MemoryUsage = uint64(memInfo.RSS)
			}
			if createTime, err := p.CreateTime(); err == nil {
				targetApp.StartTime = createTime / 1000
			}
		} else {
			// Process doesn't exist, update status
			if targetApp.Status != "STOPPED" {
				targetApp.Status = "EXITED"
				targetApp.Pid = 0
			}
		}
	}

	c.JSON(http.StatusOK, api.Response{Success: true, Data: targetApp})
}

func (s *Server) handleBindMySQL(c *gin.Context) {
	appName := c.Param("appName")

	var req provisioner.ProvisionMySQLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "Invalid request body"})
		return
	}

	result := provisioner.ProvisionMySQL(appName, req)
	if !result.Success {
		statusCode := http.StatusInternalServerError
		if result.Data != nil {
			if dataMap, ok := result.Data.(map[string]any); ok {
				if errorCode, ok := dataMap["error_code"].(string); ok && errorCode == "needs_credentials" {
					statusCode = http.StatusForbidden
				}
			}
		}
		c.JSON(statusCode, result)
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) handleBindRedis(c *gin.Context) {
	appName := c.Param("appName")

	var req provisioner.ProvisionRedisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.Response{Success: false, Message: "Invalid request body"})
		return
	}

	result := provisioner.ProvisionRedis(appName, req)
	if !result.Success {
		statusCode := http.StatusInternalServerError
		if result.Data != nil {
			if dataMap, ok := result.Data.(map[string]any); ok {
				if errorCode, ok := dataMap["error_code"].(string); ok && errorCode == "needs_credentials" {
					statusCode = http.StatusForbidden
				}
			}
		}
		c.JSON(statusCode, result)
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) handleServerInfo(c *gin.Context) {
	// Get server PID
	serverInfo := api.ServerInfo{
		PID: os.Getpid(),
	}

	// Get data directory
	dataDir, _ := configmanager.GetSystemConfig("data_dir")
	if dataDir == "" {
		dataDir = "."
	}
	if absDir, err := filepath.Abs(dataDir); err == nil {
		dataDir = absDir
	}
	serverInfo.DataDir = dataDir

	// Get log directory
	logDir := filepath.Join(dataDir, "logs")
	serverInfo.LogDir = logDir

	// Get config path
	configPath := filepath.Join(dataDir, "config.json")
	serverInfo.ConfigPath = configPath

	// Get version from git or build
	serverInfo.Version = getVersion()

	// Get uptime - get process start time
	if p, err := process.NewProcess(int32(os.Getpid())); err == nil {
		if createTime, err := p.CreateTime(); err == nil {
			// Convert milliseconds to seconds
			startTime := createTime / 1000
			uptime := time.Now().Unix() - startTime
			serverInfo.Uptime = uptime
		}
	}

	c.JSON(http.StatusOK, api.Response{Success: true, Data: serverInfo})
}

// getVersion returns the version of nova-server
func getVersion() string {
	// Try to get version from git describe
	if _, err := exec.LookPath("git"); err == nil {
		// Check if we're in a git repository
		dir, _ := os.Getwd()
		for {
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
				// Found .git, run git describe
				cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
				cmd.Dir = dir
				if output, err := cmd.Output(); err == nil {
					version := string(output)
					// Trim whitespace and 'v' prefix if present
					version = fmt.Sprintf("%s", version)
					return version
				}
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// Fallback to runtime version
	return fmt.Sprintf("%s (%s/%s)", "dev", runtime.GOOS, runtime.GOARCH)
}
