package localcommand

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExecutesCommandsInOrderAndStopsOnFailure(t *testing.T) {
	var stdout bytes.Buffer
	err := Run(context.Background(), []Command{
		{Target: "api", Action: "stop", ShellCommand: "printf stop; exit 7"},
		{Target: "api", Action: "start", ShellCommand: "printf start"},
	}, t.TempDir(), Streams{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: io.Discard})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("err = %v", err)
	}
	if stdout.String() != "[api] $ stop: printf stop; exit 7\nstop" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunUsesConfiguredDirectoryEnvironmentAndStdin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NOVA_LOCAL_TEST", "inherited")
	var stdout bytes.Buffer
	err := Run(context.Background(), []Command{{
		Target:       "default",
		Action:       "start",
		ShellCommand: "printf '%s:' \"$NOVA_LOCAL_TEST\"; pwd; read value; printf '%s' \"$value\"",
	}}, dir, Streams{Stdin: strings.NewReader("input\n"), Stdout: &stdout, Stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	want := "inherited:" + dir + "\ninput"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunReportsTargetActionAndCommandOnFailure(t *testing.T) {
	err := Run(context.Background(), []Command{{Target: "worker", Action: "stop", ShellCommand: "exit 9"}}, t.TempDir(), Streams{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
	})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v", err)
	}
	if exitErr.Target != "worker" || exitErr.Action != "stop" || exitErr.Command != "exit 9" || exitErr.ExitCode() != 9 {
		t.Fatalf("exit error = %+v", exitErr)
	}
}

func TestRunRejectsInvalidInputBeforeExecutingAnything(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "started")
	err := Run(context.Background(), []Command{
		{Target: "api", Action: "start", ShellCommand: "touch " + shellQuote(marker)},
		{Target: "worker", Action: "stop", ShellCommand: ""},
	}, dir, Streams{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "worker stop command is required") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("first command ran before preflight: %v", statErr)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
