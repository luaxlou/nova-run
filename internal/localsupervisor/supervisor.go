package localsupervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const defaultReapTimeout = 2 * time.Second

func RunSupervisor(ctx context.Context, startupPath string, lockFile, readyFile *os.File) error {
	if lockFile == nil || readyFile == nil {
		return fmt.Errorf("local supervisor inherited descriptors are required")
	}
	defer lockFile.Close()
	defer readyFile.Close()
	startup, err := readAndRemoveStartup(startupPath)
	if err != nil {
		return writeReadyError(readyFile, err)
	}
	listener, err := listenControl(startup.Paths.Socket)
	if err != nil {
		return writeReadyError(readyFile, err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(startup.Paths.Socket)
	}()
	output, err := os.OpenFile(startup.Paths.Output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return writeReadyError(readyFile, fmt.Errorf("open local supervisor output: %w", err))
	}
	defer output.Close()
	if err := output.Chmod(0o600); err != nil {
		return writeReadyError(readyFile, fmt.Errorf("secure local supervisor output: %w", err))
	}

	starting := stateForStartup(startup, PhaseStarting, os.Getpid(), 0)
	if err := WriteState(startup.Paths.State, starting); err != nil {
		return writeReadyError(readyFile, err)
	}
	cmd, err := startApplication(startup, output)
	if err != nil {
		final := finalState(starting, PhaseError, 1)
		_ = WriteState(startup.Paths.State, final)
		return writeReadyError(readyFile, err)
	}
	running := stateForStartup(startup, PhaseRunning, os.Getpid(), cmd.Process.Pid)
	if err := WriteState(startup.Paths.State, running); err != nil {
		_ = signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
		waited := make(chan error, 1)
		go func() { waited <- cmd.Wait() }()
		_, _ = waitForExit(waited, defaultReapTimeout)
		return writeReadyError(readyFile, err)
	}
	if err := writeReady(readyFile, readyMessage{OK: true, State: running}); err != nil {
		exitCode, stopErr := terminateStartedApplication(cmd, startup.StopGrace)
		final := finalState(running, PhaseError, exitCode)
		stateErr := WriteState(startup.Paths.State, final)
		return errors.Join(err, stopErr, stateErr)
	}
	return superviseApplication(ctx, listener, cmd, startup, running)
}

func readAndRemoveStartup(path string) (Startup, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Startup{}, fmt.Errorf("read local supervisor startup: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return Startup{}, fmt.Errorf("remove local supervisor startup: %w", err)
	}
	var startup Startup
	if err := json.Unmarshal(payload, &startup); err != nil {
		return Startup{}, fmt.Errorf("decode local supervisor startup: %w", err)
	}
	if startup.Schema != StateSchema || startup.Target.ProjectPath == "" || startup.Target.Name == "" || startup.Target.Dir == "" || startup.Target.Start == "" || startup.Nonce == "" {
		return Startup{}, fmt.Errorf("local supervisor startup is invalid")
	}
	if startup.StopGrace <= 0 {
		return Startup{}, fmt.Errorf("local supervisor stop grace must be positive")
	}
	return startup, nil
}

func writeReadyError(file *os.File, err error) error {
	if writeErr := writeReady(file, readyMessage{Error: err.Error()}); writeErr != nil {
		return errors.Join(err, writeErr)
	}
	return err
}

func writeReady(file *os.File, message readyMessage) error {
	if err := json.NewEncoder(file).Encode(message); err != nil {
		return fmt.Errorf("write local supervisor readiness: %w", err)
	}
	return nil
}

func stateForStartup(startup Startup, phase string, supervisorPID, appPID int) State {
	fingerprint := sha256.Sum256([]byte(startup.Target.Start))
	return State{
		Schema: StateSchema, ProjectPath: startup.Target.ProjectPath, App: startup.Target.Name,
		Phase: phase, SupervisorPID: supervisorPID, AppPID: appPID,
		CommandFingerprint: hex.EncodeToString(fingerprint[:]), StartedAt: startup.StartedAt.Format(time.RFC3339Nano),
		Nonce: startup.Nonce,
	}
}

func superviseApplication(ctx context.Context, listener net.Listener, cmd *exec.Cmd, startup Startup, initial State) error {
	var stateMu sync.RWMutex
	current := initial
	readCurrent := func() State {
		stateMu.RLock()
		defer stateMu.RUnlock()
		return current
	}
	setCurrent := func(next State) error {
		if err := WriteState(startup.Paths.State, next); err != nil {
			return err
		}
		stateMu.Lock()
		current = next
		stateMu.Unlock()
		return nil
	}
	stops := make(chan stopRequest)
	go serveControl(listener, readCurrent, stops)
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	var waitErr error
	var request *stopRequest
	requested := false
	select {
	case waitErr = <-waited:
	case item := <-stops:
		request = &item
		requested = true
	case <-signals:
		requested = true
	case <-ctx.Done():
		requested = true
	}

	if requested {
		stopping := readCurrent()
		stopping.Phase = PhaseStopping
		if err := setCurrent(stopping); err != nil {
			return err
		}
		if err := signalProcessGroup(cmd.Process.Pid, syscall.SIGTERM); err != nil {
			return err
		}
		timer := time.NewTimer(startup.StopGrace)
		select {
		case waitErr = <-waited:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			if err := signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL); err != nil {
				return err
			}
			var reapErr error
			waitErr, reapErr = waitForExit(waited, defaultReapTimeout)
			if reapErr != nil {
				return reapErr
			}
		}
	}

	exitCode := processExitCode(cmd, waitErr)
	phase := PhaseStopped
	if !requested && exitCode != 0 {
		phase = PhaseError
	}
	final := finalState(readCurrent(), phase, exitCode)
	if err := setCurrent(final); err != nil {
		return err
	}
	if request != nil {
		request.response <- final
		select {
		case <-request.delivered:
		case <-time.After(10 * time.Second):
			return fmt.Errorf("local supervisor stop acknowledgement timed out")
		}
	}
	return nil
}

func terminateStartedApplication(cmd *exec.Cmd, grace time.Duration) (int, error) {
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	termErr := signalProcessGroup(cmd.Process.Pid, syscall.SIGTERM)
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case waitErr := <-waited:
		return processExitCode(cmd, waitErr), termErr
	case <-timer.C:
		killErr := signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
		waitErr, reapErr := waitForExit(waited, defaultReapTimeout)
		return processExitCode(cmd, waitErr), errors.Join(termErr, killErr, reapErr)
	}
}

func waitForExit(waited <-chan error, timeout time.Duration) (error, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case waitErr := <-waited:
		return waitErr, nil
	case <-timer.C:
		return nil, fmt.Errorf("application process reap timed out after %s", timeout)
	}
}

func processExitCode(cmd *exec.Cmd, waitErr error) int {
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	if waitErr == nil {
		return 0
	}
	return 1
}

func finalState(state State, phase string, exitCode int) State {
	state.Phase = phase
	state.AppPID = 0
	state.SupervisorPID = 0
	state.ExitedAt = time.Now().Format(time.RFC3339Nano)
	state.ExitCode = &exitCode
	return state
}

func statusFromState(state State) Status {
	status := Status{App: state.App, State: state.Phase, PID: state.AppPID, ExitCode: state.ExitCode}
	status.StartedAt, _ = time.Parse(time.RFC3339Nano, state.StartedAt)
	status.ExitedAt, _ = time.Parse(time.RFC3339Nano, state.ExitedAt)
	return status
}
