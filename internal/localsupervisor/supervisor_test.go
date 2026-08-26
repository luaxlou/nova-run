package localsupervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunSupervisorCapturesOutputAndPersistsExit(t *testing.T) {
	target, paths, lock := lockedTestTarget(t, "printf supervised-output; exit 7")
	startupPath := writeTestStartup(t, target, paths, 200*time.Millisecond)
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- RunSupervisor(context.Background(), startupPath, lock.File(), readyWrite)
	}()
	state := readReadyState(t, readyRead)
	if state.Phase != PhaseRunning {
		t.Fatalf("ready state = %#v", state)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	state, ok, err := ReadState(paths.State)
	if err != nil || !ok || state.Phase != PhaseError || state.ExitCode == nil || *state.ExitCode != 7 {
		t.Fatalf("state=%#v ok=%v err=%v", state, ok, err)
	}
	output, err := os.ReadFile(paths.Output)
	if err != nil || string(output) != "supervised-output" {
		t.Fatalf("output=%q err=%v", output, err)
	}
}

func TestRunSupervisorSecuresExistingOutputLog(t *testing.T) {
	target, paths, lock := lockedTestTarget(t, "exit 0")
	if err := os.WriteFile(paths.Output, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	startupPath := writeTestStartup(t, target, paths, 200*time.Millisecond)
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- RunSupervisor(context.Background(), startupPath, lock.File(), readyWrite) }()
	_ = readReadyState(t, readyRead)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(paths.Output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("output mode=%v", info.Mode().Perm())
	}
}

func TestStopRequestTerminatesWholeProcessGroup(t *testing.T) {
	target, paths, state, done := startTestSupervisor(t, "sh -c 'sleep 30 & echo $! > child.pid; wait'")
	childPID := readPIDEventually(t, filepath.Join(target.Dir, "child.pid"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := requestStop(ctx, paths, state); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	waitProcessGone(t, childPID)
	final, ok, err := ReadState(paths.State)
	if err != nil || !ok || final.Phase != PhaseStopped {
		t.Fatalf("final=%#v ok=%v err=%v", final, ok, err)
	}
}

func TestQueryRejectsMismatchedNonce(t *testing.T) {
	_, paths, state, done := startTestSupervisor(t, "sleep 30")
	wrong := state
	wrong.Nonce = "wrong"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := query(ctx, paths, wrong); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("err = %v", err)
	}
	if err := requestStop(ctx, paths, state); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestStopAcknowledgedBeforeSupervisorReturns(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previousProcs)
	_, paths, state, done := startTestSupervisor(t, "sleep 30")
	stopDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		stopDone <- requestStop(ctx, paths, state)
	}()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("supervisor returned before stop acknowledgement was delivered")
	}
}

func TestReadinessFailureKillsTermIgnoringApplication(t *testing.T) {
	target, paths, lock := lockedTestTarget(t, "trap '' TERM; while :; do sleep 1; done")
	startupPath := writeTestStartup(t, target, paths, 100*time.Millisecond)
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = readyRead.Close()
	done := make(chan error, 1)
	go func() { done <- RunSupervisor(context.Background(), startupPath, lock.File(), readyWrite) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("readiness failure unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		state, ok, _ := ReadState(paths.State)
		if ok && state.AppPID > 0 {
			_ = syscall.Kill(-state.AppPID, syscall.SIGKILL)
		}
		<-done
		t.Fatal("supervisor hung after readiness pipe failure")
	}
}

func lockedTestTarget(t *testing.T, command string) (Target, Paths, *Lock) {
	t.Helper()
	dir := t.TempDir()
	target := Target{ProjectPath: filepath.Join(dir, "nova.yaml"), Name: "api", Dir: dir, Start: command}
	cacheRoot, err := os.MkdirTemp("/tmp", "nova-supervisor-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cacheRoot) })
	paths, err := PathsFor(cacheRoot, target)
	if err != nil {
		t.Fatal(err)
	}
	lock, owned, err := TryLock(paths.Lock)
	if err != nil || !owned {
		t.Fatalf("lock owned=%v err=%v", owned, err)
	}
	return target, paths, lock
}

func writeTestStartup(t *testing.T, target Target, paths Paths, grace time.Duration) string {
	t.Helper()
	startup := Startup{
		Schema: StateSchema, Target: target, Paths: paths, Nonce: "test-nonce",
		StartedAt: time.Now(), StopGrace: grace,
	}
	payload, err := json.Marshal(startup)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Startup, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return paths.Startup
}

func readReadyState(t *testing.T, ready *os.File) State {
	t.Helper()
	defer ready.Close()
	if err := ready.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(ready).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var message readyMessage
	if err := json.Unmarshal(line, &message); err != nil {
		t.Fatal(err)
	}
	if !message.OK {
		t.Fatalf("supervisor readiness failed: %s", message.Error)
	}
	return message.State
}

func startTestSupervisor(t *testing.T, command string) (Target, Paths, State, <-chan error) {
	t.Helper()
	target, paths, lock := lockedTestTarget(t, command)
	startupPath := writeTestStartup(t, target, paths, 200*time.Millisecond)
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- RunSupervisor(context.Background(), startupPath, lock.File(), readyWrite)
	}()
	state := readReadyState(t, readyRead)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = requestStop(ctx, paths, state)
	})
	return target, paths, state, done
}

func readPIDEventually(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		payload, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(payload)))
			if err != nil {
				t.Fatal(err)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pid file %s was not created", path)
	return 0
}

func waitProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(fmt.Sprintf("process %d is still present", pid))
}
