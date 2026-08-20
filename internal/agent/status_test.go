package agent

import (
	"path/filepath"
	"testing"

	"github.com/luaxlou/glow-ops/internal/deploy"
	"github.com/luaxlou/glow-ops/internal/runtime"
)

func TestResolveStatusReportsStaticArtifactAsDeployed(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "frontend")
	if err := deploy.SaveMetadata(appDir, deploy.Metadata{Version: "f9de619dc2c2"}); err != nil {
		t.Fatal(err)
	}

	status, err := resolveAppStatus(root, "frontend", func(string) (runtime.Status, error) {
		return runtime.Status{State: "inactive", SubState: "dead", PID: "0", Started: "n/a", ExitCode: "0"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "deployed" {
		t.Fatalf("state = %q", status.State)
	}
	if status.SubState != "static" {
		t.Fatalf("substate = %q", status.SubState)
	}
	if status.Version != "f9de619dc2c2" {
		t.Fatalf("version = %q", status.Version)
	}
}
