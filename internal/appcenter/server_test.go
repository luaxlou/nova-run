package appcenter

import (
	"os"
	"testing"

	"github.com/luaxlou/glow-ops/pkg/api"
)

func TestMergeAppInfo_PreservesCommandWhenIncomingIsPartial(t *testing.T) {
	existing := api.AppInfo{
		Name:        "four-server",
		Command:     "/data/apps/four-server/nova_four-server",
		WorkingDir:  "/data/apps/four-server",
		Env:         map[string]string{"A": "B"},
		Args:        []string{"--x", "1"},
		AutoRestart: true,
		Domain:      "example.com",
		Status:      "RUNNING",
		Port:        1234,
		Pid:         999,
	}

	// Simulate the common case: app reports only runtime fields, omitting Command.
	incoming := api.AppInfo{
		Name:   "four-server",
		Pid:    1000,
		Port:   4321,
		Status: "RUNNING",
		// Command intentionally empty
	}

	merged := mergeAppInfo(existing, incoming)

	if merged.Command != existing.Command {
		t.Fatalf("expected Command preserved, got %q want %q", merged.Command, existing.Command)
	}
	if merged.WorkingDir != existing.WorkingDir {
		t.Fatalf("expected WorkingDir preserved, got %q want %q", merged.WorkingDir, existing.WorkingDir)
	}
	if merged.Env == nil || merged.Env["A"] != "B" {
		t.Fatalf("expected Env preserved, got %#v", merged.Env)
	}
	if merged.Args == nil || len(merged.Args) != 2 || merged.Args[0] != "--x" {
		t.Fatalf("expected Args preserved, got %#v", merged.Args)
	}

	// Runtime fields should update.
	if merged.Pid != 1000 {
		t.Fatalf("expected Pid updated, got %d", merged.Pid)
	}
	if merged.Port != 4321 {
		t.Fatalf("expected Port updated, got %d", merged.Port)
	}
	if merged.Status != "RUNNING" {
		t.Fatalf("expected Status updated, got %q", merged.Status)
	}

	// AutoRestart should not be reset by incoming false.
	if merged.AutoRestart != true {
		t.Fatalf("expected AutoRestart preserved as true, got %v", merged.AutoRestart)
	}
}

func TestEnrichAppInfoFromPID_FillsCommandForLegacyClients(t *testing.T) {
	appInfo := api.AppInfo{
		Name: "test-app",
		Pid:  os.Getpid(),
		// Command intentionally empty (legacy client behavior)
	}

	enrichAppInfoFromPID(&appInfo)

	if appInfo.Command == "" {
		t.Fatalf("expected Command to be inferred from PID, got empty")
	}
}
