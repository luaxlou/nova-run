package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luaxlou/glow-ops/internal/localcommand"
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

func TestRunAndRestartUseTheSameLocalSequence(t *testing.T) {
	for _, action := range []string{"restart", "run"} {
		t.Run(action, func(t *testing.T) {
			dir := t.TempDir()
			raw := []byte("start: 'printf start >> events'\nstop: 'printf stop >> events'\n")
			if err := os.WriteFile(filepath.Join(dir, project.ConfigFile), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			err := runConfiguredLocalLifecycle(context.Background(), dir, action, lifecycleArgs{}, localcommand.Streams{
				Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
			})
			if err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(filepath.Join(dir, "events"))
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != "stopstart" {
				t.Fatalf("events = %q", content)
			}
		})
	}
}

func TestConfiguredLocalLifecycleRunsOnlyRequestedAction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.ConfigFile), []byte("start: printf start\nstop: printf stop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := runConfiguredLocalLifecycle(context.Background(), dir, "start", lifecycleArgs{}, localcommand.Streams{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: io.Discard,
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(stdout.String(), "\nstart") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestConfiguredLocalLifecyclePreflightsAllTargets(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("apps:\n  api:\n    start: touch api-started\n    stop: true\n  web:\n    stop: true\n")
	if err := os.WriteFile(filepath.Join(dir, project.ConfigFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := runConfiguredLocalLifecycle(context.Background(), dir, "start", lifecycleArgs{Selector: "all"}, localcommand.Streams{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "apps.web start is required") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "api-started")); !os.IsNotExist(statErr) {
		t.Fatalf("api started before preflight: %v", statErr)
	}
}

func TestRestartAllStopsEveryTargetBeforeStartingAnyTarget(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`apps:
  api:
    start: printf start-api, >> events
    stop: printf stop-api, >> events
  web:
    start: printf start-web, >> events
    stop: printf stop-web, >> events
`)
	if err := os.WriteFile(filepath.Join(dir, project.ConfigFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := runConfiguredLocalLifecycle(context.Background(), dir, "restart", lifecycleArgs{Selector: "all"}, localcommand.Streams{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "events"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "stop-api,stop-web,start-api,start-web," {
		t.Fatalf("events = %q", content)
	}
}

func TestConfiguredLocalLifecyclePreservesChildExitCode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.ConfigFile), []byte("start: exit 8\nstop: exit 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runConfiguredLocalLifecycle(context.Background(), dir, "run", lifecycleArgs{}, localcommand.Streams{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
	})
	if got := cliExitCode(err); got != 7 {
		t.Fatalf("exit code = %d, err = %v", got, err)
	}
}

type fakeRemoteLifecycleClient struct {
	calls []string
	err   error
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

func TestRemoteRunMapsToRestart(t *testing.T) {
	cli := &fakeRemoteLifecycleClient{}
	err := runConfiguredRemoteLifecycle(context.Background(), cli, "run", []project.Target{{App: "api"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cli.calls) != 1 || cli.calls[0] != "restart:api" {
		t.Fatalf("calls = %v", cli.calls)
	}
}

func TestRemoteLifecycleStopsAtFirstError(t *testing.T) {
	cli := &fakeRemoteLifecycleClient{err: errors.New("remote failed")}
	err := runConfiguredRemoteLifecycle(context.Background(), cli, "start", []project.Target{{App: "api"}, {App: "web"}})
	if err == nil || len(cli.calls) != 1 {
		t.Fatalf("err = %v calls = %v", err, cli.calls)
	}
}

func TestAutoBootstrapRuntimeConfigSkipsDefaultLocalLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"NOVA_ENDPOINT", "NOVA_AGENT_ENDPOINT", "NOVA_TOKEN", "NOVA_AGENT_TOKEN"} {
		t.Setenv(key, "")
	}
	for _, action := range []string{"start", "stop", "restart", "run"} {
		if err := autoBootstrapRuntimeConfig(action); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
}
