package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luaxlou/glow-ops/internal/localsupervisor"
	"github.com/luaxlou/glow-ops/internal/project"
)

func TestParseLifecycleArgsAcceptsRemoteAnywhere(t *testing.T) {
	for _, args := range [][]string{{"--remote", "api"}, {"api", "--remote"}} {
		got, err := parseLifecycleArgs(args)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Remote || got.Selector != "api" {
			t.Fatalf("args %v parsed as %+v", args, got)
		}
	}
}

func TestParseLifecycleArgsRejectsAmbiguousArguments(t *testing.T) {
	for _, args := range [][]string{{"api", "web"}, {"--remote", "--remote"}, {"--other"}} {
		if _, err := parseLifecycleArgs(args); err == nil {
			t.Fatalf("args %v unexpectedly accepted", args)
		}
	}
}

func TestLifecycleCommandsIncludeDefaultLocalStatus(t *testing.T) {
	for _, action := range []string{"start", "stop", "restart", "run", "status"} {
		if !isLifecycleCommand(action) {
			t.Fatalf("%s is not a lifecycle command", action)
		}
	}
}

func TestRunAndRestartUseTheSameLocalManagerOperation(t *testing.T) {
	for _, action := range []string{"restart", "run"} {
		t.Run(action, func(t *testing.T) {
			dir := writeLifecycleConfig(t, "start: sleep 30\n")
			manager := &fakeLocalLifecycleManager{}
			if err := runConfiguredLocalLifecycle(context.Background(), dir, action, lifecycleArgs{}, manager, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if len(manager.calls) != 1 || manager.calls[0] != "restart:default" {
				t.Fatalf("calls=%v", manager.calls)
			}
		})
	}
}

func TestLocalStatusDoesNotRequireStartCommand(t *testing.T) {
	dir := writeLifecycleConfig(t, "apps:\n  api: {}\n")
	manager := &fakeLocalLifecycleManager{statuses: map[string]localsupervisor.Status{
		"api": {App: "api", State: "not_started"},
	}}
	var stdout bytes.Buffer
	if err := runConfiguredLocalLifecycle(context.Background(), dir, "status", lifecycleArgs{Selector: "api"}, manager, &stdout); err != nil {
		t.Fatal(err)
	}
	if len(manager.calls) != 1 || manager.calls[0] != "status:api" {
		t.Fatalf("calls=%v", manager.calls)
	}
	if !strings.Contains(stdout.String(), "app=api state=not_started") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestConfiguredLocalLifecyclePreflightsAllTargets(t *testing.T) {
	dir := writeLifecycleConfig(t, "apps:\n  api:\n    start: sleep 30\n  web: {}\n")
	manager := &fakeLocalLifecycleManager{}
	err := runConfiguredLocalLifecycle(context.Background(), dir, "start", lifecycleArgs{Selector: "all"}, manager, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "apps.web start is required") {
		t.Fatalf("err=%v", err)
	}
	if len(manager.calls) != 0 {
		t.Fatalf("manager called before preflight: %v", manager.calls)
	}
}

func TestLocalLifecycleUsesSelectorNotRemoteAppAlias(t *testing.T) {
	dir := writeLifecycleConfig(t, `apps:
  api:
    app: remote-api
    start: sleep 30
`)
	manager := &fakeLocalLifecycleManager{}
	if err := runConfiguredLocalLifecycle(context.Background(), dir, "start", lifecycleArgs{Selector: "api"}, manager, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(manager.calls) != 1 || manager.calls[0] != "start:api" {
		t.Fatalf("calls=%v", manager.calls)
	}
}

type fakeLocalLifecycleManager struct {
	calls    []string
	statuses map[string]localsupervisor.Status
	err      error
}

func (f *fakeLocalLifecycleManager) StartAll(_ context.Context, targets []localsupervisor.Target) ([]localsupervisor.Result, error) {
	f.record("start", targets)
	return fakeResults(targets), f.err
}

func (f *fakeLocalLifecycleManager) StopAll(_ context.Context, targets []localsupervisor.Target) ([]localsupervisor.Result, error) {
	f.record("stop", targets)
	return fakeResults(targets), f.err
}

func (f *fakeLocalLifecycleManager) RestartAll(_ context.Context, targets []localsupervisor.Target) ([]localsupervisor.Result, error) {
	f.record("restart", targets)
	return fakeResults(targets), f.err
}

func (f *fakeLocalLifecycleManager) Status(_ context.Context, target localsupervisor.Target) (localsupervisor.Status, error) {
	f.calls = append(f.calls, "status:"+target.Name)
	if f.err != nil {
		return localsupervisor.Status{}, f.err
	}
	if status, ok := f.statuses[target.Name]; ok {
		return status, nil
	}
	return localsupervisor.Status{App: target.Name, State: localsupervisor.PhaseRunning}, nil
}

func (f *fakeLocalLifecycleManager) record(action string, targets []localsupervisor.Target) {
	for _, target := range targets {
		f.calls = append(f.calls, action+":"+target.Name)
	}
}

func fakeResults(targets []localsupervisor.Target) []localsupervisor.Result {
	results := make([]localsupervisor.Result, 0, len(targets))
	for _, target := range targets {
		results = append(results, localsupervisor.Result{App: target.Name, State: localsupervisor.PhaseRunning, OutputPath: "/tmp/output.log"})
	}
	return results
}

type fakeRemoteLifecycleClient struct {
	calls  []string
	status string
	err    error
}

func (f *fakeRemoteLifecycleClient) Start(_ context.Context, app string) error {
	f.calls = append(f.calls, "start:"+app)
	return f.err
}

func (f *fakeRemoteLifecycleClient) Stop(_ context.Context, app string) error {
	f.calls = append(f.calls, "stop:"+app)
	return f.err
}

func (f *fakeRemoteLifecycleClient) Restart(_ context.Context, app string) error {
	f.calls = append(f.calls, "restart:"+app)
	return f.err
}

func (f *fakeRemoteLifecycleClient) Status(_ context.Context, app string) (string, error) {
	f.calls = append(f.calls, "status:"+app)
	return f.status, f.err
}

func TestRemoteRunMapsToRestart(t *testing.T) {
	cli := &fakeRemoteLifecycleClient{}
	err := runConfiguredRemoteLifecycle(context.Background(), cli, "run", []project.Target{{App: "api"}}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cli.calls) != 1 || cli.calls[0] != "restart:api" {
		t.Fatalf("calls=%v", cli.calls)
	}
}

func TestRemoteStatusUsesAgentClient(t *testing.T) {
	cli := &fakeRemoteLifecycleClient{status: "remote-status"}
	var stdout bytes.Buffer
	if err := runConfiguredRemoteLifecycle(context.Background(), cli, "status", []project.Target{{App: "api"}}, &stdout); err != nil {
		t.Fatal(err)
	}
	if len(cli.calls) != 1 || cli.calls[0] != "status:api" || stdout.String() != "remote-status\n" {
		t.Fatalf("calls=%v stdout=%q", cli.calls, stdout.String())
	}
}

func TestRemoteLifecycleStopsAtFirstError(t *testing.T) {
	cli := &fakeRemoteLifecycleClient{err: errors.New("remote failed")}
	err := runConfiguredRemoteLifecycle(context.Background(), cli, "start", []project.Target{{App: "api"}, {App: "web"}}, &bytes.Buffer{})
	if err == nil || len(cli.calls) != 1 {
		t.Fatalf("err=%v calls=%v", err, cli.calls)
	}
}

func TestAutoBootstrapRuntimeConfigSkipsDefaultLocalLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"NOVA_ENDPOINT", "NOVA_AGENT_ENDPOINT", "NOVA_TOKEN", "NOVA_AGENT_TOKEN"} {
		t.Setenv(key, "")
	}
	for _, action := range []string{"start", "stop", "restart", "run", "status"} {
		if err := autoBootstrapRuntimeConfig(action); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
}

func writeLifecycleConfig(t *testing.T, raw string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.ConfigFile), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
