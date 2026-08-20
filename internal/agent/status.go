package agent

import (
	"os"
	"path/filepath"

	"github.com/luaxlou/glow-ops/internal/artifact"
	"github.com/luaxlou/glow-ops/internal/deploy"
	"github.com/luaxlou/glow-ops/internal/runtime"
)

func resolveAppStatus(appRoot, name string, processStatus func(string) (runtime.Status, error)) (runtime.Status, error) {
	appDir := filepath.Join(appRoot, name)
	version, hasVersion, err := deploy.CurrentVersion(appRoot, name)
	if err != nil {
		return runtime.Status{}, err
	}
	if _, err := os.Stat(appDir); err != nil {
		if os.IsNotExist(err) {
			return processStatus(name)
		}
		return runtime.Status{}, err
	}
	if !artifact.HasRunBinary(appDir) {
		status := runtime.Status{
			State:    "deployed",
			SubState: "static",
			PID:      "0",
			Started:  "n/a",
			ExitCode: "0",
		}
		if hasVersion {
			status.Version = version
		}
		return status, nil
	}
	status, err := processStatus(name)
	if err != nil {
		return runtime.Status{}, err
	}
	if hasVersion {
		status.Version = version
	}
	return status, nil
}
