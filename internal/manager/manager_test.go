package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/luaxlou/glow-ops/internal/configmanager"
	"github.com/luaxlou/glow-ops/internal/statemanager"
	"github.com/luaxlou/glow-ops/pkg/api"
	"github.com/luaxlou/glow-ops/internal/storage"
)

func initTestState(t *testing.T, tmpDir string) {
	t.Helper()
	statePath := filepath.Join(tmpDir, "nova-state.json")
	t.Cleanup(func() { _ = os.Remove(statePath) })
	storage.Init(statePath)
	storage.Reload()
}

func TestAppManager_StartApp(t *testing.T) {
	// Setup temp dir
	tmpDir := t.TempDir()
	initTestState(t, tmpDir)
	configmanager.Init()

	// Initialize Config
	if err := configmanager.EnsureInitialized(); err != nil {
		t.Fatalf("Failed to init config manager: %v", err)
	}
	configmanager.SetSystemConfig("data_dir", tmpDir)
	configmanager.SetSystemConfig("api_key", "test-key")
	configmanager.SetSystemConfig("server_url", "127.0.0.1:8080")

	// Create dummy binary (shell script)
	dummySrc := filepath.Join(tmpDir, "dummy_app")
	scriptContent := `#!/bin/sh
echo "Starting dummy app"
while true; do
  echo "running"
  sleep 1
done
`
	if err := os.WriteFile(dummySrc, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create dummy app: %v", err)
	}

	// Start App
	req := api.StartAppRequest{
		Name:        "test-app",
		Command:     dummySrc,
		AutoRestart: false,
	}

	if err := StartApp(req); err != nil {
		t.Fatalf("StartApp failed: %v", err)
	}

	// Verify Dir
	appDir := filepath.Join(tmpDir, "apps", "test-app")
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		t.Errorf("App dir not created")
	}

	// Verify Binary Renamed
	renamedBin := filepath.Join(appDir, "nova_test-app")
	if _, err := os.Stat(renamedBin); os.IsNotExist(err) {
		t.Errorf("Binary not renamed/copied")
	}

	// Verify Logs Created
	logDir := filepath.Join(appDir, "logs")
	logFile := filepath.Join(logDir, "test-app.log")
	// Allow some time for log creation
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("Log file not created")
	}

	// Verify Process Running
	time.Sleep(1 * time.Second)
	apps := ListApps()
	if len(apps) == 0 {
		t.Errorf("No apps listed")
	} else {
		found := false
		for _, app := range apps {
			if app.Name == "test-app" {
				found = true
				if app.Status != "RUNNING" {
					t.Errorf("App status is %s, expected RUNNING", app.Status)
				}
				if app.Pid == 0 {
					t.Errorf("App PID is 0")
				}
			}
		}
		if !found {
			t.Errorf("test-app not found in list")
		}
	}

	// Stop App
	if err := StopApp("test-app"); err != nil {
		t.Fatalf("StopApp failed: %v", err)
	}

	// Verify Stopped
	apps = ListApps()
	for _, app := range apps {
		if app.Name == "test-app" {
			if app.Status != "STOPPED" {
				t.Errorf("App status is %s, expected STOPPED", app.Status)
			}
		}
	}
}

func TestAppManager_StartApp_FallbackToDeployedBinaryWhenCommandMissing(t *testing.T) {
	tmpDir := t.TempDir()
	initTestState(t, tmpDir)
	configmanager.Init()

	if err := configmanager.EnsureInitialized(); err != nil {
		t.Fatalf("Failed to init config manager: %v", err)
	}
	configmanager.SetSystemConfig("data_dir", tmpDir)
	configmanager.SetSystemConfig("api_key", "test-key")
	configmanager.SetSystemConfig("server_url", "127.0.0.1:8080")

	// Pre-create deployed binary at apps/<name>/nova_<name>
	// Use a unique app name to avoid conflicts with other tests
	appName := "fallback-test-app"
	appDir := filepath.Join(tmpDir, "apps", appName)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatalf("Failed to create app dir: %v", err)
	}
	deployedBin := filepath.Join(appDir, "nova_"+appName)
	scriptContent := `#!/bin/sh
echo "Starting dummy app"
while true; do
  echo "running"
  sleep 1
done
`
	if err := os.WriteFile(deployedBin, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create deployed binary: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(deployedBin); err != nil {
		t.Fatalf("Deployed binary not found after creation: %v (path: %s)", err, deployedBin)
	}

	// Clean up any existing database records from previous test runs
	// Use statemanager.DeleteApp to only remove DB record, not filesystem
	_ = statemanager.DeleteApp(appName)

	// Start with missing Command; should fall back to deployedBin.
	req := api.StartAppRequest{
		Name:        appName,
		Command:     "",
		AutoRestart: false,
	}
	if err := StartApp(req); err != nil {
		t.Fatalf("StartApp failed: %v", err)
	}

	// Verify the app started with the correct command
	time.Sleep(200 * time.Millisecond)
	apps := ListApps()
	found := false
	for _, app := range apps {
		if app.Name == appName {
			found = true
			if app.Command != deployedBin {
				t.Errorf("Expected Command to be %s, got %s", deployedBin, app.Command)
			}
		}
	}
	if !found {
		t.Errorf("App %s not found after start", appName)
	}

	// Stop to avoid leaking process.
	if err := StopApp(appName); err != nil {
		t.Fatalf("StopApp failed: %v", err)
	}
}

func TestAppManager_StopApp_KeepIngressDoesNotRemoveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	initTestState(t, tmpDir)
	configmanager.Init()

	if err := configmanager.EnsureInitialized(); err != nil {
		t.Fatalf("Failed to init config manager: %v", err)
	}
	configmanager.SetSystemConfig("data_dir", tmpDir)
	configmanager.SetSystemConfig("api_key", "test-key")
	configmanager.SetSystemConfig("server_url", "127.0.0.1:8080")

	dummySrc := filepath.Join(tmpDir, "dummy_app_keep_ingress")
	scriptContent := `#!/bin/sh
echo "Starting dummy app"
while true; do
  echo "running"
  sleep 1
done
`
	if err := os.WriteFile(dummySrc, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create dummy app: %v", err)
	}

	appName := "keep-ingress-test-app"
	_ = statemanager.DeleteApp(appName)
	if err := statemanager.SaveApp(api.AppInfo{
		Name:   appName,
		Status: "STOPPED",
		Domain: "keep.example.com",
		Port:   18080,
	}); err != nil {
		t.Fatalf("Failed to seed app state: %v", err)
	}

	if err := StartApp(api.StartAppRequest{Name: appName, Command: dummySrc}); err != nil {
		t.Fatalf("StartApp failed: %v", err)
	}

	confFile := filepath.Join(tmpDir, "nginx", appName+".conf")
	if _, err := os.Stat(confFile); os.IsNotExist(err) {
		t.Fatalf("Nginx config file not created")
	}

	if err := StopAppWithOptions(appName, StopAppOptions{KeepIngress: true}); err != nil {
		t.Fatalf("StopAppWithOptions failed: %v", err)
	}

	if _, err := os.Stat(confFile); os.IsNotExist(err) {
		t.Fatalf("Nginx config file removed unexpectedly")
	}
}
