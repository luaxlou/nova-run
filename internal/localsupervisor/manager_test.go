package localsupervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

func TestRestartAllStopsEveryAppBeforeStartingAnyApp(t *testing.T) {
	m := testManager(t)
	events := filepath.Join(t.TempDir(), "events")
	targets := []Target{
		testTargetWithName(t, "api", eventLoopCommand(events, "api")),
		testTargetWithName(t, "web", eventLoopCommand(events, "web")),
	}
	if _, err := m.StartAll(context.Background(), targets); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.StopAll(context.Background(), targets) })
	waitForEvents(t, events, 2)
	if err := os.WriteFile(events, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RestartAll(context.Background(), targets); err != nil {
		t.Fatal(err)
	}
	want := []string{"stop-api", "stop-web", "start-api", "start-web"}
	if got := waitForEvents(t, events, len(want)); !slices.Equal(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
}

func TestStartAllRollsBackOnlyNewStarts(t *testing.T) {
	m := testManager(t)
	already := testTargetWithName(t, "already", "sleep 30")
	newApp := testTargetWithName(t, "new", "sleep 30")
	broken := testTargetWithName(t, "broken", "sleep 30")
	if _, err := m.Start(context.Background(), already); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Stop(context.Background(), already) })
	realLaunch := m.launchSupervisor
	m.launchSupervisor = func(ctx context.Context, startup Startup, lock *Lock) (State, error) {
		if startup.Target.Name == "broken" {
			return State{}, errors.New("injected readiness failure")
		}
		return realLaunch(ctx, startup, lock)
	}
	if _, err := m.StartAll(context.Background(), []Target{already, newApp, broken}); err == nil {
		t.Fatal("expected batch start failure")
	}
	if status, err := m.Status(context.Background(), already); err != nil || status.State != PhaseRunning {
		t.Fatalf("already=%#v err=%v", status, err)
	}
	if status, err := m.Status(context.Background(), newApp); err != nil || status.State == PhaseRunning {
		t.Fatalf("new=%#v err=%v", status, err)
	}
}

func TestStartAllPreflightsBeforeMutation(t *testing.T) {
	m := testManager(t)
	valid := testTargetWithName(t, "valid", "sleep 30")
	invalid := testTargetWithName(t, "invalid", "")
	if _, err := m.StartAll(context.Background(), []Target{valid, invalid}); err == nil {
		t.Fatal("expected preflight failure")
	}
	status, err := m.Status(context.Background(), valid)
	if err != nil || status.State != "not_started" {
		t.Fatalf("valid target mutated before preflight: status=%#v err=%v", status, err)
	}
}

func TestStopEscalatesToKillAfterGrace(t *testing.T) {
	m := testManager(t)
	m.StopGrace = 100 * time.Millisecond
	target := testTarget(t, "trap '' TERM; while :; do sleep 1; done")
	if _, err := m.Start(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := m.Stop(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	if elapsed < 100*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("forced stop elapsed=%v", elapsed)
	}
	status, err := m.Status(context.Background(), target)
	if err != nil || status.State != PhaseStopped {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestStatusReturnsUnknownWhenLockOwnerHasNoSocket(t *testing.T) {
	m := testManager(t)
	m.ControlTimeout = 100 * time.Millisecond
	target := testTarget(t, "sleep 30")
	paths, err := PathsFor(m.CacheRoot, target)
	if err != nil {
		t.Fatal(err)
	}
	lock, owned, err := TryLock(paths.Lock)
	if err != nil || !owned {
		t.Fatalf("owned=%v err=%v", owned, err)
	}
	defer lock.Close()
	state := State{Schema: StateSchema, ProjectPath: target.ProjectPath, App: target.Name, Phase: PhaseRunning, Nonce: "held"}
	if err := WriteState(paths.State, state); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Status(context.Background(), target); err == nil || !strings.Contains(err.Error(), "state=unknown") {
		t.Fatalf("err=%v", err)
	}
}

func TestSupervisorUsesProjectDirectoryEnvironmentAndLog(t *testing.T) {
	m := testManager(t)
	t.Setenv("NOVA_SUPERVISOR_TEST", "inherited")
	target := testTarget(t, "printf '%s:%s' \"$NOVA_SUPERVISOR_TEST\" \"$(pwd)\"")
	if _, err := m.Start(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	status := waitForPhase(t, m, target, PhaseStopped)
	if status.ExitCode == nil || *status.ExitCode != 0 {
		t.Fatalf("status=%#v", status)
	}
	paths, err := PathsFor(m.CacheRoot, target)
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(paths.Output)
	if err != nil || string(output) != "inherited:"+target.Dir {
		t.Fatalf("output=%q err=%v", output, err)
	}
}

func TestLaunchFailureReleasesLockAndCleansStartup(t *testing.T) {
	m := testManager(t)
	m.Executable = filepath.Join(t.TempDir(), "missing-nova")
	m.launchSupervisor = m.launch
	target := testTarget(t, "sleep 30")
	if _, err := m.Start(context.Background(), target); err == nil {
		t.Fatal("expected launch failure")
	}
	paths, err := PathsFor(m.CacheRoot, target)
	if err != nil {
		t.Fatal(err)
	}
	lock, owned, err := TryLock(paths.Lock)
	if err != nil || !owned {
		t.Fatalf("lock was not released: owned=%v err=%v", owned, err)
	}
	defer lock.Close()
	if _, err := os.Stat(paths.Startup); !os.IsNotExist(err) {
		t.Fatalf("startup payload remains: %v", err)
	}
}

func TestAlreadyStoppedCleansOnlyStaleControlFiles(t *testing.T) {
	m := testManager(t)
	target := testTarget(t, "")
	paths, err := PathsFor(m.CacheRoot, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.Socket, paths.Startup} {
		if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := m.Stop(context.Background(), target)
	if err != nil || !result.Already {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	for _, path := range []string{paths.Socket, paths.Startup} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale path %s remains: %v", path, err)
		}
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
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	dir = canonical
	return Target{ProjectPath: filepath.Join(dir, "nova.yaml"), Name: "api", Dir: dir, Start: command}
}

func testTargetWithName(t *testing.T, name, command string) Target {
	t.Helper()
	target := testTarget(t, command)
	target.Name = name
	return target
}

func eventLoopCommand(path, name string) string {
	return "trap 'printf stop-" + name + ", >> " + path + "; exit 0' TERM; " +
		"printf start-" + name + ", >> " + path + "; while :; do sleep 1; done"
}

func waitForEvents(t *testing.T, path string, count int) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		payload, err := os.ReadFile(path)
		if err == nil {
			parts := strings.Split(strings.TrimSuffix(string(payload), ","), ",")
			if len(parts) >= count {
				return parts
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("did not observe %d events in %s", count, path)
	return nil
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
