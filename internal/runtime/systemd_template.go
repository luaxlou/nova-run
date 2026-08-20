package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

const appServiceTemplatePath = "/etc/systemd/system/nova@.service"

func EnsureAppServiceTemplate(appRoot string) error {
	if goruntime.GOOS != "linux" {
		return nil
	}
	root := filepath.Clean(strings.TrimSpace(appRoot))
	if root == "" || root == "." {
		return fmt.Errorf("app root is required")
	}
	content := appServiceTemplate(root)
	current, err := os.ReadFile(appServiceTemplatePath)
	if err == nil && string(current) == content {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read app service template: %w", err)
	}
	if err := os.WriteFile(appServiceTemplatePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write app service template: %w", err)
	}
	cmd := exec.Command("systemctl", "daemon-reload")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("reload systemd: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func appServiceTemplate(appRoot string) string {
	return fmt.Sprintf(`[Unit]
Description=Nova App %%i
After=network.target nova.service

[Service]
Type=simple
WorkingDirectory=%s/%%i
ExecStart=%s/%%i/run
Restart=on-failure
RestartSec=2
KillSignal=SIGTERM
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
`, appRoot, appRoot)
}
