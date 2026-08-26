# Nova Run Local Supervisor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace stateless local lifecycle commands with detached per-app supervisors so local start/stop/restart/run return promptly and local status reports trustworthy runtime state.

**Architecture:** Each configured local app gets a cache-backed state directory, advisory lock, Unix control socket, and detached Nova supervisor process. The public CLI resolves all targets before mutation, starts supervisors through an inherited locked descriptor and readiness pipe, and treats `run` as an exact alias for stop-all then start-all.

**Tech Stack:** Go 1.24.5, standard library (`os/exec`, `net`, `encoding/json`, `crypto/sha256`), `golang.org/x/sys/unix`, YAML v3, Unix process groups, Go tests using real short-lived shell processes.

**Spec:** `docs/superpowers/specs/2026-08-26-local-supervisor-design.md`

## Global Constraints

- Release this breaking change as `v0.2.0`.
- Local supervision supports Linux and macOS on amd64 and arm64; Windows remains unsupported.
- There is one detached supervisor per running app and no global daemon.
- The supervisor never automatically restarts an exited application.
- `start` is the only local lifecycle command in `nova.yaml`; root or app `stop` keys are migration errors.
- `run` is an exact alias for `restart`: stop every selected app, then start every selected app.
- Local ownership is established by an advisory lock plus matching socket identity and nonce, never by PID existence alone.
- All control operations and readiness waits are bounded; graceful stop sends TERM to the process group, waits three seconds, then sends KILL.
- Runtime directories and files use `0700` and `0600`; app names are hashed before use in paths.
- Local lifecycle commands bypass Nova Agent endpoint/token bootstrap; remote behavior requires `--remote`.
- Run local Go verification with `CGO_ENABLED=0` because the development machine's Go 1.26.1 cgo path crashes in `go-m1cpu`.

## File Map

- `internal/project/config.go`: start-only lifecycle schema, deprecated-field rejection, identity resolution, ordered target preflight.
- `internal/project/config_test.go`: schema migration and lifecycle resolver tests.
- `internal/localsupervisor/types.go`: public target, paths, state, status, options, and control protocol types.
- `internal/localsupervisor/paths.go`: cache path derivation and safe hashing.
- `internal/localsupervisor/state.go`: atomic state persistence and validation.
- `internal/localsupervisor/lock_unix.go`: nonblocking advisory lock ownership on Linux/macOS.
- `internal/localsupervisor/control.go`: bounded Unix-socket request/response helpers.
- `internal/localsupervisor/process_unix.go`: detached supervisor launch and process-group signaling.
- `internal/localsupervisor/supervisor.go`: hidden child loop, readiness, app wait, final-state recording.
- `internal/localsupervisor/manager.go`: start/stop/status/restart orchestration and rollback.
- `internal/localsupervisor/*_test.go`: unit and real-process integration coverage.
- `cmd/nova/main.go`: lifecycle parsing/routing, default-local status, remote status, hidden supervisor entry.
- `cmd/nova/lifecycle_test.go`: CLI routing and run/restart equivalence tests.
- `internal/localcommand/executor.go`: delete after supervisor code replaces it.
- `internal/localcommand/executor_test.go`: delete with the obsolete implementation.
- `README.md`, `docs/nova-run-spec.md`, `CHANGELOG.md`, `VERSION`: v0.2.0 contract and migration documentation.

---

### Task 1: Convert lifecycle configuration to start-only identities

**Files:**
- Modify: `internal/project/config.go`
- Modify: `internal/project/config_test.go`

**Interfaces:**
- Produces: `type LifecycleTarget struct { Name string; Start string }`
- Produces: `ResolveLifecycle(cfg Config, selector string, requireStart bool) (LifecycleTarget, error)`
- Produces: `ResolveAllLifecycles(cfg Config, requireStart bool) ([]LifecycleTarget, error)`
- Produces: `LoadForLifecycle(dir string) (Config, string, error)` rejecting `run` and `stop` keys before typed decoding can hide empty values.

- [ ] **Step 1: Replace the old lifecycle expectations with failing start-only tests**

```go
func TestLoadForLifecycleRejectsStopField(t *testing.T) {
	for _, raw := range []string{
		"start: sleep 30\nstop: kill-app\n",
		"apps:\n  api:\n    start: sleep 30\n    stop:\n",
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := LoadForLifecycle(dir)
		if err == nil || !strings.Contains(err.Error(), "stop is no longer supported") {
			t.Fatalf("err = %v", err)
		}
	}
}

func TestResolveLifecycleAllowsIdentityOnlyLookup(t *testing.T) {
	cfg := Config{Apps: map[string]App{"api": {}}, AppOrder: []string{"api"}}
	target, err := ResolveLifecycle(cfg, "api", false)
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "api" || target.Start != "" {
		t.Fatalf("target = %#v", target)
	}
	if _, err := ResolveLifecycle(cfg, "api", true); err == nil || !strings.Contains(err.Error(), "start is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveAllLifecycleStartsPreflightInYAMLOrder(t *testing.T) {
	cfg := Config{
		Start: "root-start",
		Apps: map[string]App{"api": {}, "web": {Start: "web-start"}},
		AppOrder: []string{"web", "api"},
	}
	targets, err := ResolveAllLifecycles(cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{targets[0].Name, targets[1].Name}; !slices.Equal(got, []string{"web", "api"}) {
		t.Fatalf("order = %v", got)
	}
}
```

- [ ] **Step 2: Run the focused tests and confirm the old stop-based API fails**

Run: `CGO_ENABLED=0 go test ./internal/project -run 'TestLoadForLifecycleRejectsStopField|TestResolveLifecycleAllowsIdentityOnlyLookup|TestResolveAllLifecycleStartsPreflightInYAMLOrder' -count=1`

Expected: FAIL because `stop` is accepted and the resolver still requires action constants.

- [ ] **Step 3: Remove `Stop` fields and implement raw deprecated-key detection**

```go
type Config struct {
	App       string         `json:"app,omitempty" yaml:"app,omitempty"`
	Start     string         `json:"start,omitempty" yaml:"start,omitempty"`
	Build     BuildConfig    `json:"build,omitempty" yaml:"build,omitempty"`
	Artifacts []string       `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
	Service   ServiceConfig  `json:"service,omitempty" yaml:"service,omitempty"`
	Apps      map[string]App `json:"apps,omitempty" yaml:"apps,omitempty"`
	AppOrder  []string       `json:"-" yaml:"-"`
}

type App struct {
	App       string        `json:"app,omitempty" yaml:"app,omitempty"`
	Start     string        `json:"start,omitempty" yaml:"start,omitempty"`
	Build     BuildConfig   `json:"build,omitempty" yaml:"build,omitempty"`
	Artifacts []string      `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
	Service   ServiceConfig `json:"service,omitempty" yaml:"service,omitempty"`
}

func rejectDeprecatedLifecycleFields(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", ConfigFile, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parse %s: %w", ConfigFile, err)
	}
	return walkLifecycleKeys(root.Content, func(label, key string) error {
		switch key {
		case "run":
			return fmt.Errorf("%s run is no longer supported; configure start", label)
		case "stop":
			return fmt.Errorf("%s stop is no longer supported; Nova supervises the start process", label)
		default:
			return nil
		}
	})
}
```

Implement `walkLifecycleKeys` to inspect only the document root and mappings directly below `apps`, label them `nova.yaml` and `nova.yaml apps.<name>`, and preserve YAML declaration order through `readAppOrder`.

- [ ] **Step 4: Implement start-required versus identity-only resolution**

```go
type LifecycleTarget struct {
	Name  string
	Start string
}

func ResolveLifecycle(cfg Config, selector string, requireStart bool) (LifecycleTarget, error) {
	name, app, err := resolveLifecycleIdentity(cfg, strings.TrimSpace(selector))
	if err != nil {
		return LifecycleTarget{}, err
	}
	target := LifecycleTarget{Name: name, Start: strings.TrimSpace(firstNonEmpty(app.Start, cfg.Start))}
	if requireStart && target.Start == "" {
		label := ConfigFile
		if name != "default" {
			label += " apps." + name
		}
		return LifecycleTarget{}, fmt.Errorf("%s start is required", label)
	}
	return target, nil
}
```

Implement `ResolveAllLifecycles` by resolving every ordered app into a temporary slice and returning `nil, err` if any required start is absent, so callers cannot mutate before full preflight.

- [ ] **Step 5: Run project tests and commit**

Run: `CGO_ENABLED=0 go test ./internal/project -count=1`

Expected: PASS.

```bash
git add internal/project/config.go internal/project/config_test.go
git commit -m "feat: define start-only local lifecycle config"
```

---

### Task 2: Add safe runtime paths, atomic state, and exclusive locks

**Files:**
- Create: `internal/localsupervisor/types.go`
- Create: `internal/localsupervisor/paths.go`
- Create: `internal/localsupervisor/state.go`
- Create: `internal/localsupervisor/lock_unix.go`
- Create: `internal/localsupervisor/paths_test.go`
- Create: `internal/localsupervisor/state_test.go`
- Create: `internal/localsupervisor/lock_test.go`

**Interfaces:**
- Produces: `type Target struct { ProjectPath, Name, Dir, Start string }`
- Produces: `type Paths struct { Dir, Lock, State, Socket, Output, Startup string }`
- Produces: `PathsFor(cacheRoot string, target Target) (Paths, error)`
- Produces: `type State struct { Schema int; ProjectPath, App, Phase string; SupervisorPID, AppPID int; CommandFingerprint, StartedAt, ExitedAt string; ExitCode *int; Nonce string }`
- Produces: `WriteState(path string, state State) error` and `ReadState(path string) (State, bool, error)`
- Produces: `TryLock(path string) (*Lock, bool, error)` where `bool` reports ownership and `(*Lock).Close()` releases it.

- [ ] **Step 1: Write failing path, state, and lock tests**

```go
func TestPathsForHashesUntrustedIdentity(t *testing.T) {
	target := Target{ProjectPath: "/tmp/example/nova.yaml", Name: "../../api"}
	paths, err := PathsFor(t.TempDir(), target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(paths.Dir, "..") || filepath.Base(paths.Dir) == "api" {
		t.Fatalf("unsafe dir = %q", paths.Dir)
	}
	if filepath.Dir(paths.Lock) != paths.Dir || filepath.Dir(paths.Socket) != paths.Dir {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestStateRoundTripUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app", "state.json")
	want := State{Schema: 1, ProjectPath: "/p/nova.yaml", App: "api", Phase: PhaseRunning, Nonce: "abc"}
	if err := WriteState(path, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadState(path)
	if err != nil || !ok || got.Nonce != want.Nonce {
		t.Fatalf("state=%#v ok=%v err=%v", got, ok, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
}

func TestTryLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	first, owned, err := TryLock(path)
	if err != nil || !owned {
		t.Fatalf("owned=%v err=%v", owned, err)
	}
	defer first.Close()
	second, owned, err := TryLock(path)
	if err != nil || owned || second != nil {
		t.Fatalf("second=%v owned=%v err=%v", second, owned, err)
	}
}
```

- [ ] **Step 2: Run the new package tests and verify missing symbols fail**

Run: `CGO_ENABLED=0 go test ./internal/localsupervisor -run 'TestPathsFor|TestStateRoundTrip|TestTryLock' -count=1`

Expected: FAIL because `Target`, `PathsFor`, state persistence, and locking do not exist.

- [ ] **Step 3: Define stable runtime types and hashed paths**

```go
const (
	StateSchema  = 1
	PhaseStarting = "starting"
	PhaseRunning  = "running"
	PhaseStopping = "stopping"
	PhaseStopped  = "stopped"
	PhaseError    = "error"
)

func PathsFor(cacheRoot string, target Target) (Paths, error) {
	projectPath, err := filepath.Abs(target.ProjectPath)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve project path: %w", err)
	}
	projectID := shortHash(filepath.Clean(projectPath))
	appID := shortHash(target.Name)
	dir := filepath.Join(cacheRoot, "nova", "run", projectID, appID)
	return Paths{
		Dir: dir, Lock: filepath.Join(dir, "lock"), State: filepath.Join(dir, "state.json"),
		Socket: filepath.Join(dir, "control.sock"), Output: filepath.Join(dir, "output.log"), Startup: filepath.Join(dir, "startup.json"),
	}, nil
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}
```

Reject empty cache root, project path, or target name before path creation.

- [ ] **Step 4: Implement atomic private state writes and validated reads**

```go
func WriteState(path string, state State) error {
	state.Schema = StateSchema
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return fmt.Errorf("create state temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil { _ = tmp.Close(); return err }
	if _, err := tmp.Write(payload); err != nil { _ = tmp.Close(); return err }
	if err := tmp.Sync(); err != nil { _ = tmp.Close(); return err }
	if err := tmp.Close(); err != nil { return err }
	if err := os.Rename(tmpName, path); err != nil { return fmt.Errorf("replace state: %w", err) }
	return nil
}
```

`ReadState` returns `(State{}, false, nil)` for a missing file and rejects schema mismatches, empty identity, unknown phases, or malformed JSON.

- [ ] **Step 5: Implement nonblocking `flock` ownership**

```go
func TryLock(path string) (*Lock, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, fmt.Errorf("create lock dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil { return nil, false, fmt.Errorf("open lock: %w", err) }
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) { return nil, false, nil }
		return nil, false, fmt.Errorf("lock runtime: %w", err)
	}
	return &Lock{file: file}, true, nil
}
```

Expose `File() *os.File` for descriptor inheritance and make `Close` unlock then close exactly once.

- [ ] **Step 6: Run package tests, race the lock tests, and commit**

Run: `CGO_ENABLED=0 go test ./internal/localsupervisor -count=1`

Run: `go test -race ./internal/localsupervisor -run 'TestTryLock' -count=1`

Expected: PASS.

```bash
git add internal/localsupervisor
git commit -m "feat: add local supervisor runtime state"
```

---

### Task 3: Implement bounded control protocol and the supervisor child loop

**Files:**
- Modify: `internal/localsupervisor/types.go`
- Create: `internal/localsupervisor/control.go`
- Create: `internal/localsupervisor/process_unix.go`
- Create: `internal/localsupervisor/supervisor.go`
- Create: `internal/localsupervisor/control_test.go`
- Create: `internal/localsupervisor/supervisor_test.go`

**Interfaces:**
- Consumes: `Paths`, `State`, `WriteState`, `ReadState`, and inherited `*os.File` lock ownership from Task 2.
- Produces: `type Startup struct { Schema int; Target Target; Paths Paths; Nonce string; StartedAt time.Time; StopGrace time.Duration }`
- Produces: `RunSupervisor(ctx context.Context, startupPath string, lockFile, readyFile *os.File) error`
- Produces: `query(ctx context.Context, paths Paths, expected State) (Status, error)` and `requestStop(ctx context.Context, paths Paths, expected State) error`.

- [ ] **Step 1: Write failing protocol and real child-process tests**

```go
func TestRunSupervisorCapturesOutputAndPersistsExit(t *testing.T) {
	target, paths, lock := testLockedTarget(t, "printf supervised-output; exit 7")
	startup := writeTestStartup(t, target, paths, 200*time.Millisecond)
	readyRead, readyWrite, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	done := make(chan error, 1)
	go func() { done <- RunSupervisor(context.Background(), startup, lock.File(), readyWrite) }()
	waitReady(t, readyRead)
	if err := <-done; err != nil { t.Fatal(err) }
	state, ok, err := ReadState(paths.State)
	if err != nil || !ok || state.Phase != PhaseError || state.ExitCode == nil || *state.ExitCode != 7 {
		t.Fatalf("state=%#v ok=%v err=%v", state, ok, err)
	}
	output, err := os.ReadFile(paths.Output)
	if err != nil || string(output) != "supervised-output" { t.Fatalf("output=%q err=%v", output, err) }
}

func TestStopRequestTerminatesWholeProcessGroup(t *testing.T) {
	target, paths, running := startTestSupervisor(t, "sh -c 'sleep 30 & echo $! > child.pid; wait'")
	childPID := readPIDEventually(t, filepath.Join(target.Dir, "child.pid"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := requestStop(ctx, paths, running); err != nil { t.Fatal(err) }
	waitProcessGone(t, childPID)
}

func TestQueryRejectsMismatchedNonce(t *testing.T) {
	_, paths, running := startTestSupervisor(t, "sleep 30")
	running.Nonce = "wrong"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := query(ctx, paths, running); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run the focused tests and verify the missing supervisor fails**

Run: `CGO_ENABLED=0 go test ./internal/localsupervisor -run 'TestRunSupervisor|TestStopRequest|TestQueryRejects' -count=1`

Expected: FAIL because the control server and supervisor loop do not exist.

- [ ] **Step 3: Define newline-delimited bounded JSON control messages**

```go
type controlRequest struct {
	Schema int    `json:"schema"`
	Action string `json:"action"`
	ProjectPath string `json:"projectPath"`
	App string `json:"app"`
	Nonce string `json:"nonce"`
}

type controlResponse struct {
	Schema int    `json:"schema"`
	OK bool       `json:"ok"`
	Error string  `json:"error,omitempty"`
	State State   `json:"state"`
}
```

Use `net.Dialer.DialContext(ctx, "unix", paths.Socket)`, `SetDeadline` from the context deadline, `json.Encoder.Encode`, and `io.LimitReader(conn, 64<<10)` for responses. The server validates schema, canonical project path, app, and nonce before returning status or accepting stop.

- [ ] **Step 4: Start the application in its own process group and signal the group**

```go
func startApplication(startup Startup, output *os.File) (*exec.Cmd, error) {
	cmd := exec.Command("sh", "-lc", startup.Target.Start)
	cmd.Dir = startup.Target.Dir
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil { return nil, fmt.Errorf("start application: %w", err) }
	return cmd, nil
}

func signalProcessGroup(pid int, signal syscall.Signal) error {
	if pid <= 0 { return fmt.Errorf("invalid application pid %d", pid) }
	if err := syscall.Kill(-pid, signal); err != nil && !errors.Is(err, syscall.ESRCH) { return err }
	return nil
}
```

The hidden supervisor itself is detached by its parent with `SysProcAttr.Setsid = true`; only the application uses a separate process group owned by the supervisor.

- [ ] **Step 5: Implement `RunSupervisor` readiness, wait, stop, and final state**

```go
func RunSupervisor(ctx context.Context, startupPath string, lockFile, readyFile *os.File) error {
	startup, err := readAndRemoveStartup(startupPath)
	if err != nil { return writeReadyError(readyFile, err) }
	defer lockFile.Close()
	listener, err := listenControl(startup.Paths.Socket)
	if err != nil { return writeReadyError(readyFile, err) }
	defer func() { listener.Close(); os.Remove(startup.Paths.Socket) }()
	output, err := os.OpenFile(startup.Paths.Output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil { return writeReadyError(readyFile, err) }
	defer output.Close()
	cmd, err := startApplication(startup, output)
	if err != nil { return writeReadyError(readyFile, err) }
	state := runningState(startup, os.Getpid(), cmd.Process.Pid)
	if err := WriteState(startup.Paths.State, state); err != nil { _ = signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL); return writeReadyError(readyFile, err) }
	if err := writeReadyOK(readyFile, state); err != nil { _ = signalProcessGroup(cmd.Process.Pid, syscall.SIGTERM) }
	return superviseApplication(ctx, listener, cmd, startup, state)
}
```

`superviseApplication` must have exactly one goroutine call `cmd.Wait()`. It serves status requests concurrently, turns the first validated stop or supervisor TERM/INT into TERM, waits `startup.StopGrace`, sends KILL if the wait channel has not completed, persists `stopped` for exit code 0 or requested termination and `error` for an unrequested nonzero exit, then acknowledges stop only after final state is durable.

- [ ] **Step 6: Run supervisor tests repeatedly and commit**

Run: `CGO_ENABLED=0 go test ./internal/localsupervisor -run 'TestRunSupervisor|TestStopRequest|TestQueryRejects' -count=10`

Expected: PASS ten consecutive times with no leaked `sleep` processes.

```bash
git add internal/localsupervisor
git commit -m "feat: supervise local application process groups"
```

---

### Task 4: Add detached launch, idempotent start, stop, and trustworthy status

**Files:**
- Modify: `internal/localsupervisor/types.go`
- Modify: `internal/localsupervisor/process_unix.go`
- Create: `internal/localsupervisor/manager.go`
- Create: `internal/localsupervisor/manager_test.go`

**Interfaces:**
- Consumes: supervisor runtime and protocol from Tasks 2-3.
- Produces: `type Manager struct { CacheRoot, Executable string; StartTimeout, ControlTimeout, StopGrace time.Duration; launchSupervisor launchFunc }`, where the unexported function is initialized by `NewManager` and overridden only by failure-injection tests.
- Produces: `NewManager() (*Manager, error)`
- Produces: `(*Manager).Start(context.Context, Target) (Result, error)`
- Produces: `(*Manager).Stop(context.Context, Target) (Result, error)`
- Produces: `(*Manager).Status(context.Context, Target) (Status, error)`
- Produces: `type Result struct { App, State, OutputPath string; Already bool }` and stable `Status.Line()` output.

- [ ] **Step 1: Write failing manager integration tests**

```go
func TestManagerStartReturnsPromptlyAndIsIdempotent(t *testing.T) {
	m := testManager(t)
	target := testTarget(t, "sleep 30")
	started := time.Now()
	first, err := m.Start(context.Background(), target)
	if err != nil { t.Fatal(err) }
	defer m.Stop(context.Background(), target)
	if time.Since(started) > 2*time.Second || first.State != PhaseRunning { t.Fatalf("result=%#v elapsed=%v", first, time.Since(started)) }
	second, err := m.Start(context.Background(), target)
	if err != nil || !second.Already { t.Fatalf("result=%#v err=%v", second, err) }
}

func TestManagerStatusUsesFinalStateWhenLockIsFree(t *testing.T) {
	m := testManager(t)
	target := testTarget(t, "exit 9")
	if _, err := m.Start(context.Background(), target); err != nil { t.Fatal(err) }
	waitForPhase(t, m, target, PhaseError)
	status, err := m.Status(context.Background(), target)
	if err != nil || status.State != PhaseError || status.ExitCode == nil || *status.ExitCode != 9 {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestManagerStatusReportsNotStarted(t *testing.T) {
	m := testManager(t)
	status, err := m.Status(context.Background(), testTarget(t, ""))
	if err != nil || status.State != "not_started" { t.Fatalf("status=%#v err=%v", status, err) }
}
```

The real detached-manager tests use the current test binary as a subprocess. Add this helper entry so descriptor inheritance and detachment are tested without installing a Nova binary:

```go
func TestSupervisorProcess(t *testing.T) {
	if os.Getenv("NOVA_TEST_SUPERVISOR") != "1" { return }
	args := argsAfterDoubleDash(os.Args)
	if len(args) != 2 || args[0] != "__nova_supervisor" { os.Exit(2) }
	lock := os.NewFile(uintptr(3), "test-lock")
	ready := os.NewFile(uintptr(4), "test-ready")
	if err := RunSupervisor(context.Background(), args[1], lock, ready); err != nil { os.Exit(1) }
	os.Exit(0)
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	m := &Manager{
		CacheRoot: t.TempDir(), Executable: os.Args[0],
		StartTimeout: 2*time.Second, ControlTimeout: 2*time.Second, StopGrace: 200*time.Millisecond,
	}
	m.launchSupervisor = func(ctx context.Context, startup Startup, lock *Lock) (State, error) {
		args := []string{"-test.run=^TestSupervisorProcess$", "--", "__nova_supervisor", startup.Paths.Startup}
		return m.launchCommand(ctx, startup, lock, args, []string{"NOVA_TEST_SUPERVISOR=1"})
	}
	return m
}
```

- [ ] **Step 2: Run manager tests and verify detached orchestration is absent**

Run: `CGO_ENABLED=0 go test ./internal/localsupervisor -run 'TestManager' -count=1`

Expected: FAIL because `Manager` does not exist.

- [ ] **Step 3: Implement startup payload and detached child launch**

```go
func (m *Manager) launch(ctx context.Context, startup Startup, lock *Lock) (State, error) {
	if err := writeStartup(startup.Paths.Startup, startup); err != nil { return State{}, err }
	readyRead, readyWrite, err := os.Pipe()
	if err != nil { return State{}, err }
	defer readyRead.Close()
	cmd := exec.Command(m.Executable, "__nova_supervisor", startup.Paths.Startup)
	cmd.ExtraFiles = []*os.File{lock.File(), readyWrite}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil { _ = readyWrite.Close(); return State{}, err }
	_ = readyWrite.Close()
	_ = cmd.Process.Release()
	return readReady(ctx, readyRead)
}
```

Use a `0600` startup JSON file and remove it in the child immediately after decoding. The file contains the command while the command line contains only the startup path.

Factor the descriptor/readiness work into `launchCommand(ctx, startup, lock, args, extraEnv)` so production passes `[]string{"__nova_supervisor", startup.Paths.Startup}` with no extra environment and tests pass the helper arguments above. `NewManager` assigns `launchSupervisor = m.launch`; `Start` calls that field, allowing deterministic readiness failures to be injected after earlier apps have started.

- [ ] **Step 4: Implement idempotent `Start` with inherited lock transfer**

```go
func (m *Manager) Start(ctx context.Context, target Target) (Result, error) {
	if err := validateStartTarget(target); err != nil { return Result{}, err }
	paths, err := PathsFor(m.CacheRoot, target)
	if err != nil { return Result{}, err }
	lock, owned, err := TryLock(paths.Lock)
	if err != nil { return Result{}, err }
	if !owned {
		state, err := m.liveState(ctx, paths, target)
		if err != nil { return Result{}, err }
		return Result{App: target.Name, State: state.Phase, OutputPath: paths.Output, Already: true}, nil
	}
	nonce, err := randomNonce()
	if err != nil { lock.Close(); return Result{}, err }
	startup := newStartup(target, paths, nonce, m.StopGrace)
	state, err := m.launch(withTimeout(ctx, m.StartTimeout), startup, lock)
	_ = lock.Close()
	if err != nil { cleanupFailedStart(paths); return Result{}, fmt.Errorf("start %s: %w", target.Name, err) }
	return Result{App: target.Name, State: state.Phase, OutputPath: paths.Output}, nil
}
```

The parent's `Close` drops only its descriptor; the child retains the inherited locked open-file description. If readiness fails, wait for the lock to become acquirable before returning and remove only the stale socket/startup payload.

- [ ] **Step 5: Implement idempotent `Stop` and ownership-based `Status`**

```go
func (m *Manager) Status(ctx context.Context, target Target) (Status, error) {
	paths, err := PathsFor(m.CacheRoot, target)
	if err != nil { return Status{}, err }
	lock, owned, err := TryLock(paths.Lock)
	if err != nil { return Status{}, err }
	if owned {
		_ = lock.Close()
		state, ok, err := ReadState(paths.State)
		if err != nil { return Status{}, err }
		if !ok { return notStartedStatus(target), nil }
		if err := validateFinalIdentity(state, target); err != nil { return Status{}, err }
		return statusFromState(state), nil
	}
	state, ok, err := ReadState(paths.State)
	if err != nil || !ok { return Status{}, fmt.Errorf("state=unknown app=%s: locked runtime has no valid state", target.Name) }
	live, err := query(withTimeout(ctx, m.ControlTimeout), paths, state)
	if err != nil { return Status{}, fmt.Errorf("state=unknown app=%s: %w", target.Name, err) }
	return live, nil
}
```

`Stop` uses the same lock check. When free, it removes only `control.sock` and `startup.json` and returns `Already: true`; when held, it reads and validates state, sends the bounded stop request, then polls for lock release until the control deadline. It never sends a signal from the recorded PID.

- [ ] **Step 6: Format the stable status line**

```go
func (s Status) Line() string {
	pid, started, exited, exit := "-", "-", "-", "-"
	if s.PID > 0 { pid = strconv.Itoa(s.PID) }
	if !s.StartedAt.IsZero() { started = s.StartedAt.Format(time.RFC3339) }
	if !s.ExitedAt.IsZero() { exited = s.ExitedAt.Format(time.RFC3339) }
	if s.ExitCode != nil { exit = strconv.Itoa(*s.ExitCode) }
	return fmt.Sprintf("app=%s state=%s pid=%s started=%s exited=%s exit=%s", s.App, s.State, pid, started, exited, exit)
}
```

- [ ] **Step 7: Run manager tests repeatedly and commit**

Run: `CGO_ENABLED=0 go test ./internal/localsupervisor -run 'TestManager' -count=10`

Expected: PASS with every temporary supervisor stopped by test cleanup.

```bash
git add internal/localsupervisor
git commit -m "feat: manage detached local supervisors"
```

---

### Task 5: Add restart/run ordering and multi-app rollback

**Files:**
- Modify: `internal/localsupervisor/manager.go`
- Modify: `internal/localsupervisor/manager_test.go`

**Interfaces:**
- Consumes: `Manager.Start`, `Manager.Stop`, and preflighted ordered `[]Target`.
- Produces: `(*Manager).StartAll(context.Context, []Target) ([]Result, error)`
- Produces: `(*Manager).StopAll(context.Context, []Target) ([]Result, error)`
- Produces: `(*Manager).RestartAll(context.Context, []Target) ([]Result, error)`.

- [ ] **Step 1: Write failing ordering and rollback tests**

```go
func TestRestartAllStopsEveryAppBeforeStartingAnyApp(t *testing.T) {
	m := testManager(t)
	events := filepath.Join(t.TempDir(), "events")
	targets := []Target{
		testTargetWithName(t, "api", "trap 'echo stop-api >> "+events+"; exit 0' TERM; echo start-api >> "+events+"; while :; do sleep 1; done"),
		testTargetWithName(t, "web", "trap 'echo stop-web >> "+events+"; exit 0' TERM; echo start-web >> "+events+"; while :; do sleep 1; done"),
	}
	if _, err := m.StartAll(context.Background(), targets); err != nil { t.Fatal(err) }
	truncateFile(t, events)
	defer m.StopAll(context.Background(), targets)
	if _, err := m.RestartAll(context.Background(), targets); err != nil { t.Fatal(err) }
	want := []string{"stop-api", "stop-web", "start-api", "start-web"}
	if got := readLinesEventually(t, events, 4); !slices.Equal(got, want) { t.Fatalf("events=%v", got) }
}

func TestStartAllRollsBackOnlyNewStarts(t *testing.T) {
	m := testManager(t)
	already := testTargetWithName(t, "already", "sleep 30")
	newApp := testTargetWithName(t, "new", "sleep 30")
	broken := testTargetWithName(t, "broken", "sleep 30")
	if _, err := m.Start(context.Background(), already); err != nil { t.Fatal(err) }
	defer m.Stop(context.Background(), already)
	realLaunch := m.launchSupervisor
	m.launchSupervisor = func(ctx context.Context, startup Startup, lock *Lock) (State, error) {
		if startup.Target.Name == "broken" { _ = lock.Close(); return State{}, errors.New("injected readiness failure") }
		return realLaunch(ctx, startup, lock)
	}
	if _, err := m.StartAll(context.Background(), []Target{already, newApp, broken}); err == nil { t.Fatal("expected failure") }
	if status, _ := m.Status(context.Background(), already); status.State != PhaseRunning { t.Fatalf("already=%#v", status) }
	if status, _ := m.Status(context.Background(), newApp); status.State == PhaseRunning { t.Fatalf("new=%#v", status) }
}
```

- [ ] **Step 2: Run the tests and verify orchestration methods are missing**

Run: `CGO_ENABLED=0 go test ./internal/localsupervisor -run 'TestRestartAll|TestStartAllRollsBack' -count=1`

Expected: FAIL because batch orchestration does not exist.

- [ ] **Step 3: Implement stop-all/start-all semantics and selective rollback**

```go
func (m *Manager) RestartAll(ctx context.Context, targets []Target) ([]Result, error) {
	if err := validateStartTargets(targets); err != nil { return nil, err }
	if _, err := m.StopAll(ctx, targets); err != nil { return nil, err }
	return m.StartAll(ctx, targets)
}

func (m *Manager) StartAll(ctx context.Context, targets []Target) ([]Result, error) {
	if err := validateStartTargets(targets); err != nil { return nil, err }
	results := make([]Result, 0, len(targets))
	newTargets := make([]Target, 0, len(targets))
	for _, target := range targets {
		result, err := m.Start(ctx, target)
		if err != nil {
			for i := len(newTargets)-1; i >= 0; i-- { _, _ = m.Stop(context.Background(), newTargets[i]) }
			return nil, fmt.Errorf("start all at %s: %w", target.Name, err)
		}
		results = append(results, result)
		if !result.Already { newTargets = append(newTargets, target) }
	}
	return results, nil
}
```

`StopAll` operates in input order and returns immediately on the first ownership uncertainty; `validateStartTargets` checks every name, canonical project path, working directory, and nonempty start string before any stop or start occurs.

- [ ] **Step 4: Run batch tests repeatedly and commit**

Run: `CGO_ENABLED=0 go test ./internal/localsupervisor -run 'TestRestartAll|TestStartAllRollsBack' -count=10`

Expected: PASS.

```bash
git add internal/localsupervisor/manager.go internal/localsupervisor/manager_test.go
git commit -m "feat: orchestrate local supervisor restarts"
```

---

### Task 6: Route all lifecycle commands through the local supervisor by default

**Files:**
- Modify: `cmd/nova/main.go`
- Modify: `cmd/nova/lifecycle_test.go`

**Interfaces:**
- Consumes: project lifecycle resolution and `localsupervisor.Manager`.
- Produces: `status` as a lifecycle command accepted by `parseLifecycleArgs`.
- Produces: `runConfiguredLocalLifecycle(ctx, dir, action, parsed, manager, stdout) error`.
- Extends: `remoteLifecycleClient` with `Status(context.Context, string) (string, error)`.
- Produces hidden entry: `nova __nova_supervisor <startup-path>` using inherited file descriptors 3 (lock) and 4 (readiness).

- [ ] **Step 1: Replace synchronous command tests with failing routing tests**

```go
func TestLifecycleCommandsIncludeDefaultLocalStatus(t *testing.T) {
	for _, action := range []string{"start", "stop", "restart", "run", "status"} {
		if !isLifecycleCommand(action) { t.Fatalf("%s is not lifecycle", action) }
	}
}

func TestAutoBootstrapSkipsAllDefaultLocalLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"NOVA_ENDPOINT", "NOVA_AGENT_ENDPOINT", "NOVA_TOKEN", "NOVA_AGENT_TOKEN"} { t.Setenv(key, "") }
	for _, action := range []string{"start", "stop", "restart", "run", "status"} {
		if err := autoBootstrapRuntimeConfig(action); err != nil { t.Fatalf("%s: %v", action, err) }
	}
}

func TestRemoteStatusUsesAgentClient(t *testing.T) {
	cli := &fakeRemoteLifecycleClient{status: "remote-status"}
	var out bytes.Buffer
	if err := runConfiguredRemoteLifecycle(context.Background(), cli, "status", []project.Target{{App: "api"}}, &out); err != nil { t.Fatal(err) }
	if cli.calls[0] != "status:api" || out.String() != "remote-status\n" { t.Fatalf("calls=%v out=%q", cli.calls, out.String()) }
}
```

Add an injectable fake local manager implementing:

```go
type localLifecycleManager interface {
	StartAll(context.Context, []localsupervisor.Target) ([]localsupervisor.Result, error)
	StopAll(context.Context, []localsupervisor.Target) ([]localsupervisor.Result, error)
	RestartAll(context.Context, []localsupervisor.Target) ([]localsupervisor.Result, error)
	Status(context.Context, localsupervisor.Target) (localsupervisor.Status, error)
}
```

- [ ] **Step 2: Run CLI lifecycle tests and verify status is still remote-only**

Run: `CGO_ENABLED=0 go test ./cmd/nova -run 'TestLifecycle|TestAutoBootstrap|TestRemoteStatus' -count=1`

Expected: FAIL because `status` bypasses lifecycle parsing and manager injection is absent.

- [ ] **Step 3: Add hidden supervisor routing before public command parsing**

```go
func main() {
	if len(os.Args) >= 2 && os.Args[1] == "__nova_supervisor" {
		if err := runSupervisorChild(os.Args[2:]); err != nil { os.Exit(1) }
		return
	}
	// existing help and public command routing follows
}

func runSupervisorChild(args []string) error {
	if len(args) != 1 { return fmt.Errorf("supervisor startup path required") }
	lock := os.NewFile(uintptr(3), "nova-supervisor-lock")
	ready := os.NewFile(uintptr(4), "nova-supervisor-ready")
	if lock == nil || ready == nil { return fmt.Errorf("supervisor descriptors unavailable") }
	defer ready.Close()
	return localsupervisor.RunSupervisor(context.Background(), args[0], lock, ready)
}
```

Do not document the hidden command in usage output.

- [ ] **Step 4: Route default local lifecycle without Agent bootstrap**

```go
func isLifecycleCommand(action string) bool {
	switch action {
	case "start", "stop", "restart", "run", "status": return true
	default: return false
	}
}

func lifecycleNeedsStart(action string) bool {
	return action == "start" || action == "restart" || action == "run"
}

func lifecycleTargets(dir, selector string, requireStart bool) ([]localsupervisor.Target, string, error) {
	cfg, path, err := project.LoadForLifecycle(dir)
	if err != nil { return nil, "", err }
	var resolved []project.LifecycleTarget
	if selector == "all" { resolved, err = project.ResolveAllLifecycles(cfg, requireStart) } else {
		var target project.LifecycleTarget
		target, err = project.ResolveLifecycle(cfg, selector, requireStart)
		resolved = []project.LifecycleTarget{target}
	}
	if err != nil { return nil, "", err }
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil { return nil, "", err }
	targets := make([]localsupervisor.Target, 0, len(resolved))
	for _, item := range resolved { targets = append(targets, localsupervisor.Target{ProjectPath: canonical, Name: item.Name, Dir: filepath.Dir(canonical), Start: item.Start}) }
	return targets, canonical, nil
}
```

Map `start` to `StartAll`, `stop` to `StopAll`, `restart` and `run` to the same `RestartAll` call, and `status` to ordered individual status calls. Print the canonical config path once, start result lines including `logs=<output path>`, and status via `Status.Line()`.

- [ ] **Step 5: Preserve remote lifecycle and add explicit remote status**

```go
type remoteLifecycleClient interface {
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Restart(context.Context, string) error
	Status(context.Context, string) (string, error)
}
```

`runConfiguredRemoteLifecycle` maps `run` to `Restart`, maps `status` to `Status` and writes the returned line, and keeps the existing Agent target resolution. Accept `--remote` before or after the selector with the existing duplicate/unknown/multiple argument errors.

- [ ] **Step 6: Run CLI and client tests and commit**

Run: `CGO_ENABLED=0 go test ./cmd/nova ./internal/client -count=1`

Expected: PASS.

```bash
git add -f cmd/nova/lifecycle_test.go
git add cmd/nova/main.go
git commit -m "feat: default lifecycle and status to local supervisors"
```

---

### Task 7: Exercise failure paths, forced kill, and race-sensitive behavior

**Files:**
- Modify: `internal/localsupervisor/supervisor_test.go`
- Modify: `internal/localsupervisor/manager_test.go`
- Modify: `cmd/nova/lifecycle_test.go`

**Interfaces:**
- Consumes: complete supervisor implementation.
- Produces: regression coverage for readiness failure, stale files, unresponsive owner, forced KILL, environment/working directory, logs, spontaneous success, and Agent bootstrap bypass.

- [ ] **Step 1: Add failing forced-kill and trustworthy-unknown tests**

```go
func TestStopEscalatesToKillAfterGrace(t *testing.T) {
	m := testManagerWithGrace(t, 100*time.Millisecond)
	target := testTarget(t, "trap '' TERM; while :; do sleep 1; done")
	if _, err := m.Start(context.Background(), target); err != nil { t.Fatal(err) }
	started := time.Now()
	if _, err := m.Stop(context.Background(), target); err != nil { t.Fatal(err) }
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond || elapsed > 2*time.Second { t.Fatalf("elapsed=%v", elapsed) }
	status, err := m.Status(context.Background(), target)
	if err != nil || status.State != PhaseStopped { t.Fatalf("status=%#v err=%v", status, err) }
}

func TestStatusReturnsUnknownWhenLockOwnerHasNoSocket(t *testing.T) {
	m := testManager(t)
	target := testTarget(t, "sleep 30")
	paths, err := PathsFor(m.CacheRoot, target)
	if err != nil { t.Fatal(err) }
	lock, owned, err := TryLock(paths.Lock)
	if err != nil || !owned { t.Fatalf("owned=%v err=%v", owned, err) }
	defer lock.Close()
	state := State{ProjectPath: target.ProjectPath, App: target.Name, Phase: PhaseRunning, Nonce: "held"}
	if err := WriteState(paths.State, state); err != nil { t.Fatal(err) }
	_, err = m.Status(context.Background(), target)
	if err == nil || !strings.Contains(err.Error(), "state=unknown") { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Add startup/environment/log/final-success coverage**

```go
func TestSupervisorUsesProjectDirectoryEnvironmentAndLog(t *testing.T) {
	m := testManager(t)
	t.Setenv("NOVA_SUPERVISOR_TEST", "inherited")
	target := testTarget(t, "printf '%s:%s' \"$NOVA_SUPERVISOR_TEST\" \"$(pwd)\"")
	if _, err := m.Start(context.Background(), target); err != nil { t.Fatal(err) }
	waitForPhase(t, m, target, PhaseStopped)
	paths, _ := PathsFor(m.CacheRoot, target)
	output, err := os.ReadFile(paths.Output)
	if err != nil || string(output) != "inherited:"+target.Dir { t.Fatalf("output=%q err=%v", output, err) }
}
```

Also add explicit tests that a missing shell command fails readiness without leaving a held lock, stale socket/startup files are cleaned only when the lock is free, a successful spontaneous exit persists `stopped` with exit 0, and start-all validates all targets before stopping or starting any app.

- [ ] **Step 3: Run the focused tests before any fixes**

Run: `CGO_ENABLED=0 go test ./internal/localsupervisor ./cmd/nova -run 'TestStopEscalates|TestStatusReturnsUnknown|TestSupervisorUses|TestStartAll' -count=1`

Expected: Any uncovered edge case fails with a specific state, timeout, or cleanup mismatch.

- [ ] **Step 4: Tighten the implementation until every bounded failure path passes**

Use these exact invariants while fixing failures:

```go
const (
	defaultStartTimeout   = 5 * time.Second
	defaultControlTimeout = 5 * time.Second
	defaultStopGrace      = 3 * time.Second
	maxControlMessage     = 64 << 10
)
```

Every context timeout must wrap its operation in the returned error; every cleanup path must leave `state.json` intact; no cleanup path may remove a lock file while another process may hold its inode; and only a validated control connection may trigger process-group signaling.

- [ ] **Step 5: Run repeat and race suites and commit**

Run: `CGO_ENABLED=0 go test ./internal/localsupervisor ./cmd/nova -count=20`

Run: `go test -race ./internal/localsupervisor ./cmd/nova -count=1`

Expected: PASS without flakes, data races, or leaked child processes.

```bash
git add internal/localsupervisor cmd/nova/lifecycle_test.go
git commit -m "test: harden local supervisor failure handling"
```

---

### Task 8: Remove stateless execution and document the v0.2.0 migration

**Files:**
- Delete: `internal/localcommand/executor.go`
- Delete: `internal/localcommand/executor_test.go`
- Modify: `README.md`
- Modify: `docs/nova-run-spec.md`
- Modify: `CHANGELOG.md`
- Modify: `VERSION`

**Interfaces:**
- Removes: `internal/localcommand` and all `stop:` configuration examples.
- Documents: local default lifecycle/status, `--remote`, per-app supervisor state, output log location, foreground `start`, and no automatic restart.
- Produces: version string `0.2.0` and changelog entry `v0.2.0`.

- [ ] **Step 1: Write documentation acceptance searches and observe old contract matches**

Run: `rg -n 'stop:|stateless|nova status \[app\|all\](?!.*--remote)' README.md docs/nova-run-spec.md CHANGELOG.md --pcre2`

Expected: Matches the v0.1.14 stop-command/stateless documentation that must be migrated; historical changelog text may remain only when clearly scoped to v0.1.14.

- [ ] **Step 2: Remove the obsolete package and imports**

Delete both `internal/localcommand` files and verify no production or test import remains:

Run: `rg -n 'internal/localcommand|localcommand\.' --glob '*.go'`

Expected: no output.

- [ ] **Step 3: Update user-facing configuration and command examples**

Use this canonical example in README and the product spec:

```yaml
apps:
  api:
    start: exec ./bin/api
  web:
    start: exec npm run dev
```

Document these exact semantics:

```text
nova start [app|all]              # start a detached local supervisor
nova stop [app|all]               # stop the supervised process group
nova restart [app|all]            # stop all selected apps, then start all
nova run [app|all]                # exact alias for restart
nova status [app|all]             # local supervisor status
nova <lifecycle> [app|all] --remote  # Nova Agent operation
```

State that `start` must remain in the foreground, stdout/stderr append to the printed `output.log`, local `stop` keys are rejected, no automatic restart occurs, and `nova logs` remains remote-only.

- [ ] **Step 4: Set release metadata and changelog**

Set `VERSION` to exactly:

```text
0.2.0
```

Add a `v0.2.0` changelog entry describing the breaking removal of local `stop`, the new per-app supervisor, local status default, run/restart equivalence, and `--remote` migration.

- [ ] **Step 5: Run doc checks, formatting, full verification, and cross-builds**

Run: `gofmt -w cmd/nova/main.go cmd/nova/lifecycle_test.go internal/project/config.go internal/project/config_test.go internal/localsupervisor/*.go`

Run: `git diff --check`

Run: `CGO_ENABLED=0 go vet ./...`

Run: `CGO_ENABLED=0 go test ./... -count=1`

Run: `go test -race ./internal/localsupervisor ./cmd/nova ./internal/project -count=1`

Run: `for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do GOOS=${target%/*} GOARCH=${target#*/} CGO_ENABLED=0 go build -trimpath -o /tmp/nova-${target%/*}-${target#*/} ./cmd/nova || exit 1; done`

Expected: every command exits 0 and produces four binaries.

- [ ] **Step 6: Commit the release-ready implementation**

```bash
git add -A
git commit -m "docs: prepare v0.2.0 supervisor release"
```

---

### Task 9: Review, merge, tag, and verify the public release

**Files:**
- Review only: complete branch diff against `main`
- GitHub release assets: `nova-linux-amd64`, `nova-linux-arm64`, `nova-darwin-amd64`, `nova-darwin-arm64`, `SHA256SUMS.txt`

**Interfaces:**
- Consumes: fully verified `codex/local-status` branch.
- Produces: main branch commit, signed-off tag `v0.2.0`, successful Release workflow, and verified downloadable checksums.

- [ ] **Step 1: Run final branch verification from a clean tree**

Run: `git status --short`

Expected: no output.

Run: `CGO_ENABLED=0 go test ./... -count=1 && CGO_ENABLED=0 go vet ./...`

Expected: PASS.

- [ ] **Step 2: Review the complete change and migration surface**

Run: `git diff --stat main...HEAD && git log --oneline main..HEAD`

Run: `git diff --check main...HEAD`

Expected: only supervisor/config/CLI/tests/docs/version changes, with no whitespace errors or unrelated edits.

- [ ] **Step 3: Merge the feature branch into main without discarding user work**

From `/Users/john/projects/nova-run`, first confirm `git status --short` is clean. Then run:

```bash
git merge --ff-only codex/local-status
```

Expected: main advances to the verified release commit without a merge conflict.

- [ ] **Step 4: Push main and create the release tag**

```bash
git push origin main
git tag -a v0.2.0 -m "nova-run v0.2.0"
git push origin v0.2.0
```

Expected: both pushes succeed and the tag triggers `.github/workflows/release.yml`.

- [ ] **Step 5: Wait for the Release workflow and verify assets**

Run: `gh run list --workflow Release --limit 5`

Run after the tag push: `release_run_id=$(gh run list --workflow Release --branch v0.2.0 --limit 1 --json databaseId --jq '.[0].databaseId') && gh run watch "$release_run_id" --exit-status`

Expected: workflow completes successfully.

Run: `gh release view v0.2.0 --json url,assets,tagName`

Expected: the tag is `v0.2.0` and all four binaries plus `SHA256SUMS.txt` are present.

- [ ] **Step 6: Verify published digests and report the release URL**

Download the small checksum manifest and compare its five recorded names against the release asset list:

```bash
release_dir=$(mktemp -d)
gh release download v0.2.0 --pattern SHA256SUMS.txt --dir "$release_dir"
sed -n '1,10p' "$release_dir/SHA256SUMS.txt"
```

Expected: four SHA-256 entries, one for each platform binary. Report the GitHub release URL returned by `gh release view`.
