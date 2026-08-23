package localrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, []Command{{
			Name:         "api",
			ShellCommand: `trap 'printf stopped; exit 0' INT; sleep 100 & child=$!; printf ready; wait "$child"`,
		}}, dir, Streams{Stdout: &stdout, Stderr: &stdout}, 500*time.Millisecond)
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
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, []Command{{
			Name:         "api",
			ShellCommand: `trap '' TERM; printf ready; while :; do sleep 1; done`,
		}}, dir, Streams{Stdout: &stdout, Stderr: &stdout}, 50*time.Millisecond)
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

func TestRunCleansDescendantAfterShellLeaderExits(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "child.pid")
	command := fmt.Sprintf("sleep 100 & echo $! > %s; exit 0", pidPath)
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	err = Run(context.Background(), []Command{{Name: "api", ShellCommand: command}}, dir, Streams{
		Stdout: devNull,
		Stderr: devNull,
	}, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	rawPID, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(rawPID)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	deadline := time.Now().Add(500 * time.Millisecond)
	for processExists(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processExists(pid) {
		t.Fatalf("background descendant %d survived nova run", pid)
	}
}

func TestRunForwardsIncomingUnixSignalAndUsesConventionalExitCode(t *testing.T) {
	tests := []struct {
		name       string
		signal     syscall.Signal
		wantOutput string
		wantCode   int
	}{
		{name: "interrupt", signal: syscall.SIGINT, wantOutput: "got-int", wantCode: 130},
		{name: "terminate", signal: syscall.SIGTERM, wantOutput: "got-term", wantCode: 143},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout lockedBuffer
			dir := t.TempDir()
			done := make(chan error, 1)
			go func() {
				done <- Run(context.Background(), []Command{{
					Name: "api",
					ShellCommand: `trap 'printf got-int; exit 0' INT; trap 'printf got-term; exit 0' TERM; ` +
						`sleep 100 & child=$!; printf ready; wait "$child"`,
				}}, dir, Streams{Stdout: &stdout, Stderr: &stdout}, 500*time.Millisecond)
			}()

			waitForOutput(t, &stdout, "ready")
			if err := syscall.Kill(os.Getpid(), test.signal); err != nil {
				t.Fatal(err)
			}

			select {
			case err := <-done:
				var exitErr *ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != test.wantCode {
					t.Fatalf("err = %#v, want code %d", err, test.wantCode)
				}
				output := stdout.String()
				if !strings.Contains(output, test.wantOutput) {
					t.Fatalf("output %q does not contain %q", output, test.wantOutput)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Run did not handle incoming signal")
			}
		})
	}
}

func TestRunPreservesSignalTerminatedCommandExitCode(t *testing.T) {
	err := Run(context.Background(), []Command{{
		Name: "api", ShellCommand: "kill -TERM $$",
	}}, t.TempDir(), Streams{Stdout: &lockedBuffer{}, Stderr: &lockedBuffer{}}, 100*time.Millisecond)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 143 {
		t.Fatalf("err = %#v, want code 143", err)
	}
}

func TestRunPrioritizesQueuedSignalOverChildResult(t *testing.T) {
	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGINT
	err := runWithSignals(context.Background(), []Command{{
		Name: "api", ShellCommand: "exit 7",
	}}, t.TempDir(), Streams{Stdout: &lockedBuffer{}, Stderr: &lockedBuffer{}}, 100*time.Millisecond, signals)

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 130 {
		t.Fatalf("err = %#v, want queued SIGINT code 130", err)
	}
}

func TestReapAfterKillReturnsWhenLeaderCannotBeReaped(t *testing.T) {
	started := time.Now()
	err := reapAfterKill(make(chan processResult), make([]bool, 1), 1, 20*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for 1 local process") {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("post-kill reap was not bounded: %s", elapsed)
	}
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
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
