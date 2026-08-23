package localrun

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestRunSingleConnectsStdioAndPreservesExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []Command{{
		Name:         "api",
		ShellCommand: `read value; printf 'out:%s' "$value"; printf 'err' >&2; exit 7`,
	}}, t.TempDir(), Streams{
		Stdin:  strings.NewReader("hello\n"),
		Stdout: &stdout,
		Stderr: &stderr,
	}, 100*time.Millisecond)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("err = %#v", err)
	}
	if stdout.String() != "out:hello" || stderr.String() != "err" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunAllStopsRemainingProcessAfterFirstExit(t *testing.T) {
	var stdout lockedBuffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := Run(ctx, []Command{
		{Name: "api", ShellCommand: "sleep 0.3; exit 9"},
		{Name: "web", ShellCommand: `trap 'printf stopped; exit 0' TERM; printf ready; while :; do sleep 1; done`},
	}, t.TempDir(), Streams{Stdout: &stdout, Stderr: &stdout}, 500*time.Millisecond)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 9 {
		t.Fatalf("err = %#v", err)
	}
	output := stdout.String()
	for _, want := range []string{"[api] $ sleep 0.3; exit 9", "[web] $ trap", "ready", "stopped"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestRunCancellationStopsProcessAndReturnsInterruptCode(t *testing.T) {
	var stdout lockedBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, []Command{{
			Name:         "api",
			ShellCommand: `trap 'printf stopped; exit 0' TERM; printf ready; while :; do sleep 1; done`,
		}}, t.TempDir(), Streams{Stdout: &stdout, Stderr: &stdout}, 500*time.Millisecond)
	}()

	waitForOutput(t, &stdout, "ready")
	cancel()

	select {
	case err := <-done:
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 130 {
			t.Fatalf("err = %#v", err)
		}
		if !strings.Contains(stdout.String(), "stopped") {
			t.Fatalf("output = %q", stdout.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
}

func TestRunForceKillsProcessThatIgnoresTermination(t *testing.T) {
	var stdout lockedBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, []Command{{
			Name:         "api",
			ShellCommand: `trap '' TERM; printf ready; while :; do sleep 1; done`,
		}}, t.TempDir(), Streams{Stdout: &stdout, Stderr: &stdout}, 50*time.Millisecond)
	}()

	waitForOutput(t, &stdout, "ready")
	started := time.Now()
	cancel()

	select {
	case err := <-done:
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 130 {
			t.Fatalf("err = %#v", err)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("forced cleanup took %s", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not force-kill the process")
	}
}

func waitForOutput(t *testing.T, output *lockedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("output %q does not contain %q", output.String(), want)
}
