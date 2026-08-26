package localsupervisor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSupervisorProcess(t *testing.T) {
	if os.Getenv("NOVA_TEST_SUPERVISOR") != "1" {
		return
	}
	args := argsAfterDoubleDash(os.Args)
	if len(args) != 2 || args[0] != "__nova_supervisor" {
		os.Exit(2)
	}
	lock := os.NewFile(uintptr(3), "test-lock")
	ready := os.NewFile(uintptr(4), "test-ready")
	if err := RunSupervisor(context.Background(), args[1], lock, ready); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestManagerStartReturnsPromptlyAndIsIdempotent(t *testing.T) {
	m := testManager(t)
	target := testTarget(t, "sleep 30")
	started := time.Now()
	first, err := m.Start(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Stop(context.Background(), target) })
	if elapsed := time.Since(started); elapsed > 2*time.Second || first.State != PhaseRunning || first.Already {
		t.Fatalf("result=%#v elapsed=%v", first, elapsed)
	}
	second, err := m.Start(context.Background(), target)
	if err != nil || !second.Already || second.State != PhaseRunning {
		t.Fatalf("result=%#v err=%v", second, err)
	}
}

func TestManagerStopIsIdempotent(t *testing.T) {
	m := testManager(t)
	target := testTarget(t, "sleep 30")
	if _, err := m.Start(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	first, err := m.Stop(context.Background(), target)
	if err != nil || first.Already || first.State != PhaseStopped {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := m.Stop(context.Background(), target)
	if err != nil || !second.Already || second.State != PhaseStopped {
		t.Fatalf("second=%#v err=%v", second, err)
	}
}

func TestManagerStatusUsesFinalStateWhenLockIsFree(t *testing.T) {
	m := testManager(t)
	target := testTarget(t, "exit 9")
	if _, err := m.Start(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	status := waitForPhase(t, m, target, PhaseError)
	if status.ExitCode == nil || *status.ExitCode != 9 {
		t.Fatalf("status=%#v", status)
	}
}

func TestManagerStatusReportsNotStarted(t *testing.T) {
	m := testManager(t)
	target := testTarget(t, "")
	status, err := m.Status(context.Background(), target)
	if err != nil || status.State != "not_started" || status.App != target.Name {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestStatusLineIsStable(t *testing.T) {
	exitCode := 7
	status := Status{
		App: "api", State: PhaseError, PID: 123,
		StartedAt: time.Date(2026, 8, 26, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60)),
		ExitedAt:  time.Date(2026, 8, 26, 10, 1, 0, 0, time.FixedZone("CST", 8*60*60)),
		ExitCode:  &exitCode,
	}
	want := "app=api state=error pid=123 started=2026-08-26T10:00:00+08:00 exited=2026-08-26T10:01:00+08:00 exit=7"
	if got := status.Line(); got != want {
		t.Fatalf("line=%q want=%q", got, want)
	}
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	cacheRoot, err := os.MkdirTemp("/tmp", "nova-manager-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cacheRoot) })
	m := &Manager{
		CacheRoot: cacheRoot, Executable: os.Args[0],
		StartTimeout: 2 * time.Second, ControlTimeout: 2 * time.Second, StopGrace: 200 * time.Millisecond,
	}
	m.launchSupervisor = func(ctx context.Context, startup Startup, lock *Lock) (State, error) {
		args := []string{"-test.run=^TestSupervisorProcess$", "--", "__nova_supervisor", startup.Paths.Startup}
		return m.launchCommand(ctx, startup, lock, args, []string{"NOVA_TEST_SUPERVISOR=1"})
	}
	return m
}

func testTarget(t *testing.T, command string) Target {
	t.Helper()
	dir := t.TempDir()
	return Target{ProjectPath: filepath.Join(dir, "nova.yaml"), Name: "api", Dir: dir, Start: command}
}

func waitForPhase(t *testing.T, manager *Manager, target Target, phase string) Status {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last Status
	var lastErr error
	for time.Now().Before(deadline) {
		last, lastErr = manager.Status(context.Background(), target)
		if lastErr == nil && last.State == phase {
			return last
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("phase %q not observed: status=%#v err=%v", phase, last, lastErr)
	return Status{}
}

func argsAfterDoubleDash(args []string) []string {
	for index, arg := range args {
		if strings.TrimSpace(arg) == "--" {
			return args[index+1:]
		}
	}
	return nil
}
