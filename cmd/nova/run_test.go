package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/luaxlou/glow-ops/internal/localrun"
	"github.com/luaxlou/glow-ops/internal/project"
)

func TestRunConfiguredLocalUsesRunOnlyConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.ConfigFile), []byte("run: printf local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := runConfiguredLocal(context.Background(), dir, nil, localrun.Streams{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	if output := stdout.String(); !strings.Contains(output, "project config:") || !strings.Contains(output, "local") {
		t.Fatalf("stdout = %q", output)
	}
}

func TestRunConfiguredLocalRunsAllSelectedApps(t *testing.T) {
	dir := t.TempDir()
	config := `apps:
  api:
    run: "printf api; sleep 0.2"
  web:
    run: "printf web; sleep 0.2"
`
	if err := os.WriteFile(filepath.Join(dir, project.ConfigFile), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout lockedWriter
	err := runConfiguredLocal(context.Background(), dir, []string{"all"}, localrun.Streams{
		Stdout: &stdout,
		Stderr: &stdout,
	})
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	for _, want := range []string{"[api] $", "[web] $", "api", "web"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout %q does not contain %q", output, want)
		}
	}
}

func TestRunConfiguredLocalRejectsMultipleSelectorsBeforeLoadingConfig(t *testing.T) {
	err := runConfiguredLocal(context.Background(), t.TempDir(), []string{"api", "web"}, localrun.Streams{
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "expected zero or one configured app selector") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunConfiguredLocalRejectsMissingRunBeforeStartingApps(t *testing.T) {
	dir := t.TempDir()
	config := `apps:
  api:
    run: printf-api
  worker: {}
`
	if err := os.WriteFile(filepath.Join(dir, project.ConfigFile), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := runConfiguredLocal(context.Background(), dir, []string{"all"}, localrun.Streams{
		Stdout: &stdout,
		Stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "nova.yaml apps.worker run is required") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(stdout.String(), "[api] $") {
		t.Fatalf("a process was started before preflight completed: %q", stdout.String())
	}
}

func TestLocalRunExitCodePreservesRunnerCode(t *testing.T) {
	err := &localrun.ExitError{Name: "api", Command: "exit 7", Code: 7, Err: errors.New("exit status 7")}
	if got := localRunExitCode(err); got != 7 {
		t.Fatalf("exit code = %d", got)
	}
	if got := localRunExitCode(errors.Join(errors.New("cleanup"), err)); got != 7 {
		t.Fatalf("joined exit code = %d", got)
	}
	if got := localRunExitCode(errors.New("config failed")); got != 1 {
		t.Fatalf("fallback exit code = %d", got)
	}
}

func TestAutoBootstrapRuntimeConfigSkipsLocalRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"NOVA_ENDPOINT", "NOVA_AGENT_ENDPOINT", "NOVA_TOKEN", "NOVA_AGENT_TOKEN"} {
		t.Setenv(key, "")
	}
	if err := autoBootstrapRuntimeConfig("run"); err != nil {
		t.Fatal(err)
	}
}

type lockedWriter struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}
