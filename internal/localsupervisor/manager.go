package localsupervisor

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	defaultStartTimeout   = 5 * time.Second
	defaultControlTimeout = 5 * time.Second
	defaultStopGrace      = 3 * time.Second
)

type launchFunc func(context.Context, Startup, *Lock) (State, error)

type Manager struct {
	CacheRoot        string
	Executable       string
	StartTimeout     time.Duration
	ControlTimeout   time.Duration
	StopGrace        time.Duration
	launchSupervisor launchFunc
	discoverPorts    func(int) []int
}

type Result struct {
	App        string
	State      string
	OutputPath string
	Already    bool
}

func NewManager() (*Manager, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve local supervisor cache: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve nova executable: %w", err)
	}
	m := &Manager{
		CacheRoot: cacheRoot, Executable: executable,
		StartTimeout: defaultStartTimeout, ControlTimeout: defaultControlTimeout, StopGrace: defaultStopGrace,
	}
	m.launchSupervisor = m.launch
	m.discoverPorts = discoverListeningPorts
	return m, nil
}

func (m *Manager) Start(ctx context.Context, target Target) (Result, error) {
	if err := validateTarget(target, true); err != nil {
		return Result{}, err
	}
	if err := m.validate(); err != nil {
		return Result{}, err
	}
	paths, err := PathsFor(m.CacheRoot, target)
	if err != nil {
		return Result{}, err
	}
	lock, owned, err := TryLock(paths.Lock)
	if err != nil {
		return Result{}, err
	}
	if !owned {
		state, ok, err := ReadState(paths.State)
		if err != nil || !ok {
			if err == nil {
				err = fmt.Errorf("locked runtime has no state")
			}
			return Result{}, fmt.Errorf("start %s: state=unknown: %w", target.Name, err)
		}
		if err := validateStateIdentity(state, target); err != nil {
			return Result{}, err
		}
		controlCtx, cancel := context.WithTimeout(ctx, m.ControlTimeout)
		defer cancel()
		live, err := query(controlCtx, paths, state)
		if err != nil {
			return Result{}, fmt.Errorf("start %s: state=unknown: %w", target.Name, err)
		}
		if live.State != PhaseRunning {
			return Result{}, fmt.Errorf("start %s: state=unknown: supervisor is %s", target.Name, live.State)
		}
		return Result{App: target.Name, State: live.State, OutputPath: paths.Output, Already: true}, nil
	}

	nonce, err := randomNonce()
	if err != nil {
		_ = lock.Close()
		return Result{}, err
	}
	startup := Startup{
		Schema: StateSchema, Target: target, Paths: paths, Nonce: nonce,
		StartedAt: time.Now(), StopGrace: m.StopGrace,
	}
	startCtx, cancel := context.WithTimeout(ctx, m.StartTimeout)
	state, launchErr := m.launchSupervisor(startCtx, startup, lock)
	cancel()
	closeErr := lock.Close()
	if launchErr != nil {
		cleanupStaleRuntime(paths)
		return Result{}, fmt.Errorf("start %s: %w", target.Name, launchErr)
	}
	if closeErr != nil {
		return Result{}, closeErr
	}
	return Result{App: target.Name, State: state.Phase, OutputPath: paths.Output}, nil
}

func (m *Manager) Stop(ctx context.Context, target Target) (Result, error) {
	if err := validateTarget(target, false); err != nil {
		return Result{}, err
	}
	if err := m.validate(); err != nil {
		return Result{}, err
	}
	paths, err := PathsFor(m.CacheRoot, target)
	if err != nil {
		return Result{}, err
	}
	lock, owned, err := TryLock(paths.Lock)
	if err != nil {
		return Result{}, err
	}
	if owned {
		_ = lock.Close()
		cleanupStaleRuntime(paths)
		state, ok, err := ReadState(paths.State)
		if err != nil {
			return Result{}, err
		}
		phase := PhaseStopped
		if ok {
			if err := validateStateIdentity(state, target); err != nil {
				return Result{}, err
			}
			phase = state.Phase
		}
		return Result{App: target.Name, State: phase, OutputPath: paths.Output, Already: true}, nil
	}
	state, ok, err := ReadState(paths.State)
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("locked runtime has no state")
		}
		return Result{}, fmt.Errorf("stop %s: state=unknown: %w", target.Name, err)
	}
	if err := validateStateIdentity(state, target); err != nil {
		return Result{}, err
	}
	controlCtx, cancel := context.WithTimeout(ctx, m.ControlTimeout)
	defer cancel()
	if err := requestStop(controlCtx, paths, state); err != nil {
		return Result{}, fmt.Errorf("stop %s: %w", target.Name, err)
	}
	if err := waitForLockRelease(controlCtx, paths.Lock); err != nil {
		return Result{}, fmt.Errorf("stop %s: %w", target.Name, err)
	}
	return Result{App: target.Name, State: PhaseStopped, OutputPath: paths.Output}, nil
}

func (m *Manager) Status(ctx context.Context, target Target) (Status, error) {
	if err := validateTarget(target, false); err != nil {
		return Status{}, err
	}
	if err := m.validate(); err != nil {
		return Status{}, err
	}
	paths, err := PathsFor(m.CacheRoot, target)
	if err != nil {
		return Status{}, err
	}
	lock, owned, err := TryLock(paths.Lock)
	if err != nil {
		return Status{}, err
	}
	if owned {
		_ = lock.Close()
		state, ok, err := ReadState(paths.State)
		if err != nil {
			return Status{}, err
		}
		if !ok {
			return Status{App: target.Name, State: "not_started"}, nil
		}
		if err := validateStateIdentity(state, target); err != nil {
			return Status{}, err
		}
		if state.Phase != PhaseStopped && state.Phase != PhaseError {
			return Status{}, fmt.Errorf("state=unknown app=%s: lock is free but recorded state is %s", target.Name, state.Phase)
		}
		return statusFromState(state), nil
	}
	state, ok, err := ReadState(paths.State)
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("locked runtime has no state")
		}
		return Status{}, fmt.Errorf("state=unknown app=%s: %w", target.Name, err)
	}
	if err := validateStateIdentity(state, target); err != nil {
		return Status{}, err
	}
	controlCtx, cancel := context.WithTimeout(ctx, m.ControlTimeout)
	defer cancel()
	live, err := query(controlCtx, paths, state)
	if err != nil {
		return Status{}, fmt.Errorf("state=unknown app=%s: %w", target.Name, err)
	}
	if live.PID > 0 {
		discoverPorts := m.discoverPorts
		if discoverPorts == nil {
			discoverPorts = discoverListeningPorts
		}
		live.Ports = discoverPorts(live.PID)
	}
	return live, nil
}

func (m *Manager) StartAll(ctx context.Context, targets []Target) ([]Result, error) {
	if err := validateTargets(targets, true); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(targets))
	newTargets := make([]Target, 0, len(targets))
	for _, target := range targets {
		result, err := m.Start(ctx, target)
		if err != nil {
			rollbackErrs := []error{fmt.Errorf("start all at %s: %w", target.Name, err)}
			for index := len(newTargets) - 1; index >= 0; index-- {
				if _, stopErr := m.Stop(context.Background(), newTargets[index]); stopErr != nil {
					rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback %s: %w", newTargets[index].Name, stopErr))
				}
			}
			return nil, errors.Join(rollbackErrs...)
		}
		results = append(results, result)
		if !result.Already {
			newTargets = append(newTargets, target)
		}
	}
	return results, nil
}

func (m *Manager) StopAll(ctx context.Context, targets []Target) ([]Result, error) {
	if err := validateTargets(targets, false); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(targets))
	for _, target := range targets {
		result, err := m.Stop(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("stop all at %s: %w", target.Name, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (m *Manager) RestartAll(ctx context.Context, targets []Target) ([]Result, error) {
	if err := validateTargets(targets, true); err != nil {
		return nil, err
	}
	if _, err := m.StopAll(ctx, targets); err != nil {
		return nil, err
	}
	return m.StartAll(ctx, targets)
}

func (m *Manager) launch(ctx context.Context, startup Startup, lock *Lock) (State, error) {
	return m.launchCommand(ctx, startup, lock, []string{"__nova_supervisor", startup.Paths.Startup}, nil)
}

func (m *Manager) launchCommand(ctx context.Context, startup Startup, lock *Lock, args, extraEnv []string) (State, error) {
	if err := writeStartup(startup.Paths.Startup, startup); err != nil {
		return State{}, err
	}
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		return State{}, fmt.Errorf("create local supervisor readiness pipe: %w", err)
	}
	defer readyRead.Close()
	cmd := exec.Command(m.Executable, args...)
	cmd.ExtraFiles = []*os.File{lock.File(), readyWrite}
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = readyWrite.Close()
		return State{}, fmt.Errorf("launch local supervisor: %w", err)
	}
	_ = readyWrite.Close()
	state, err := readReady(ctx, readyRead)
	if err != nil {
		_ = cmd.Process.Kill()
		waited := make(chan error, 1)
		go func() { waited <- cmd.Wait() }()
		_, _ = waitForExit(waited, defaultReapTimeout)
		return State{}, err
	}
	if err := cmd.Process.Release(); err != nil {
		return State{}, fmt.Errorf("detach local supervisor: %w", err)
	}
	return state, nil
}

func writeStartup(path string, startup Startup) error {
	if startup.Schema != StateSchema || startup.Nonce == "" || startup.StopGrace <= 0 {
		return fmt.Errorf("local supervisor startup is invalid")
	}
	payload, err := json.Marshal(startup)
	if err != nil {
		return fmt.Errorf("encode local supervisor startup: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create local supervisor runtime dir: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("secure local supervisor runtime dir: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write local supervisor startup: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func readReady(ctx context.Context, file *os.File) (State, error) {
	if deadline, ok := ctx.Deadline(); ok {
		if err := file.SetReadDeadline(deadline); err != nil {
			return State{}, fmt.Errorf("set local supervisor readiness deadline: %w", err)
		}
	}
	line, err := bufio.NewReader(file).ReadBytes('\n')
	if err != nil {
		return State{}, fmt.Errorf("wait for local supervisor readiness: %w", err)
	}
	var message readyMessage
	if err := json.Unmarshal(line, &message); err != nil {
		return State{}, fmt.Errorf("decode local supervisor readiness: %w", err)
	}
	if !message.OK {
		if message.Error == "" {
			message.Error = "startup rejected"
		}
		return State{}, fmt.Errorf("local supervisor readiness: %s", message.Error)
	}
	return message.State, nil
}

func waitForLockRelease(ctx context.Context, path string) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		lock, owned, err := TryLock(path)
		if err != nil {
			return err
		}
		if owned {
			return lock.Close()
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for local supervisor lock release: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func randomNonce() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("create local supervisor nonce: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func validateTarget(target Target, requireStart bool) error {
	if strings.TrimSpace(target.ProjectPath) == "" || !filepath.IsAbs(target.ProjectPath) {
		return fmt.Errorf("local supervisor canonical project path is required")
	}
	if strings.TrimSpace(target.Name) == "" {
		return fmt.Errorf("local supervisor app name is required")
	}
	if strings.TrimSpace(target.Dir) == "" || !filepath.IsAbs(target.Dir) {
		return fmt.Errorf("local supervisor working directory is required")
	}
	if requireStart && strings.TrimSpace(target.Start) == "" {
		return fmt.Errorf("local supervisor start command is required for %s", target.Name)
	}
	return nil
}

func validateTargets(targets []Target, requireStart bool) error {
	if len(targets) == 0 {
		return fmt.Errorf("at least one local supervisor target is required")
	}
	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		if err := validateTarget(target, requireStart); err != nil {
			return err
		}
		identity := filepath.Clean(target.ProjectPath) + "\x00" + target.Name
		if seen[identity] {
			return fmt.Errorf("duplicate local supervisor target %s", target.Name)
		}
		seen[identity] = true
	}
	return nil
}

func validateStateIdentity(state State, target Target) error {
	if filepath.Clean(state.ProjectPath) != filepath.Clean(target.ProjectPath) || state.App != target.Name || state.Nonce == "" {
		return fmt.Errorf("local supervisor state identity mismatch for %s", target.Name)
	}
	return nil
}

func (m *Manager) validate() error {
	if strings.TrimSpace(m.CacheRoot) == "" || strings.TrimSpace(m.Executable) == "" {
		return fmt.Errorf("local supervisor manager paths are required")
	}
	if m.StartTimeout <= 0 || m.ControlTimeout <= 0 || m.StopGrace <= 0 || m.launchSupervisor == nil {
		return fmt.Errorf("local supervisor manager timeouts and launcher are required")
	}
	return nil
}

func cleanupStaleRuntime(paths Paths) {
	_ = os.Remove(paths.Socket)
	_ = os.Remove(paths.Startup)
}
