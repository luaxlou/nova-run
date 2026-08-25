# Nova Run Local Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `nova start`, `nova stop`, and `nova restart` manage detached local `run` commands by default while preserving the current remote lifecycle behind `--remote`.

**Architecture:** Add a focused `internal/locallifecycle` package that starts one detached Nova supervisor per configured app, stores locked state under the user cache directory, and performs bounded process-group cleanup. Route lifecycle commands before runtime bootstrap, using a small argument parser to select the new local path or the unchanged Nova Agent path.

**Tech Stack:** Go 1.24.5, Unix process groups/sessions and advisory file locks, `sh -lc`, existing YAML project resolver, GitHub Actions release workflow.

**Spec:** `docs/superpowers/specs/2026-08-25-local-lifecycle-design.md`

## Global Constraints

- `nova run` remains foreground-only and retains its current signal and exit-code behavior.
- Local lifecycle data must use `os.UserCacheDir()` and must not dirty the project repository.
- `--remote` must work before or after the optional selector.
- Local `start`, `stop`, and `restart` must not read or bootstrap Agent credentials.
- All startup, shutdown, and cleanup waits must be bounded.
- The state lock, not a PID file alone, is the source of truth for active ownership.
- Do not add automatic restart, health checks, local `status`/`logs`, log rotation, Windows support, or a machine-wide daemon.
- Do not add a third-party dependency; use the standard library and Unix syscalls already compatible with macOS/Linux.
- The release target is `v0.1.14`; publishing is complete only after all four binaries and `SHA256SUMS.txt` are downloadable.

## File Map

- Modify `internal/project/config.go`: resolve local app identities without requiring a `run` command, while keeping `ResolveRun` validation unchanged.
- Modify `internal/project/config_test.go`: lock down default, named, `all`, and stop-without-run identity behavior.
- Create `internal/locallifecycle/types.go`: public package types, options, results, and validation.
- Create `internal/locallifecycle/state.go`: canonical project keys, safe app keys, payload/state JSON, atomic state writes, and cache layout.
- Create `internal/locallifecycle/lock_unix.go`: nonblocking advisory locks.
- Create `internal/locallifecycle/process_unix.go`: detached supervisor session and application process-group operations.
- Create `internal/locallifecycle/supervisor.go`: private supervisor protocol, readiness acknowledgement, log ownership, signal handling, and cleanup.
- Create `internal/locallifecycle/lifecycle.go`: start/stop/restart orchestration and `all` rollback.
- Create `internal/locallifecycle/state_test.go`: path safety, isolation, and atomic metadata tests.
- Create `internal/locallifecycle/lifecycle_test.go`: real detached-process lifecycle tests.
- Modify `cmd/nova/main.go`: parse lifecycle flags, route local commands before bootstrap, expose the private supervisor entry point, and preserve remote calls.
- Create `cmd/nova/lifecycle_test.go`: CLI parsing/routing tests.
- Modify `cmd/nova/run_test.go`: ensure all local commands skip remote bootstrap.
- Modify `README.md`, `docs/nova-run-spec.md`, `CHANGELOG.md`, and `VERSION`: document the breaking default and release version.

---

### Task 1: Resolve Local Lifecycle Identities

**Files:**
- Modify: `internal/project/config.go`
- Modify: `internal/project/config_test.go`

**Interfaces:**
- Produces: `type LocalIdentity struct { Name string }`
- Produces: `func ResolveLocalIdentity(cfg Config, selector string) (LocalIdentity, error)`
- Produces: `func ResolveAllLocalIdentities(cfg Config) ([]LocalIdentity, error)`
- Preserves: `ResolveRun` and `ResolveAllRuns` continue requiring resolved `run` commands.

- [ ] **Step 1: Write failing identity tests**

Add table-driven tests showing that a root project resolves to `default`, a named app resolves to its YAML selector rather than remote `app`, empty selection chooses the first declared app, `all` preserves YAML declaration order, and an app without `run` still has a stoppable identity:

```go
func TestResolveLocalIdentityDoesNotRequireRun(t *testing.T) {
	cfg := Config{Apps: map[string]App{"worker": {App: "remote-worker"}}, AppOrder: []string{"worker"}}
	got, err := ResolveLocalIdentity(cfg, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "worker" {
		t.Fatalf("name = %q, want worker", got.Name)
	}
}

func TestResolveAllLocalIdentitiesPreservesDeclarationOrder(t *testing.T) {
	cfg := Config{Apps: map[string]App{"web": {}, "api": {}}, AppOrder: []string{"api", "web"}}
	got, err := ResolveAllLocalIdentities(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "api" || got[1].Name != "web" {
		t.Fatalf("identities = %+v, want api then web", got)
	}
}
```

- [ ] **Step 2: Verify the tests fail for missing APIs**

Run: `go test ./internal/project -run 'TestResolve(LocalIdentity|AllLocalIdentities)' -count=1`

Expected: FAIL because `LocalIdentity` and resolver functions do not exist.

- [ ] **Step 3: Implement identity resolution by reusing selector order**

Add the exact type and functions, keeping root state independent of a remote app alias:

```go
type LocalIdentity struct {
	Name string
}

func ResolveLocalIdentity(cfg Config, selector string) (LocalIdentity, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		if hasDefaultProjectConfig(cfg) {
			return LocalIdentity{Name: "default"}, nil
		}
		name, _, ok := firstConfiguredApp(cfg)
		if !ok {
			return LocalIdentity{}, fmt.Errorf("%s default app is not configured", ConfigFile)
		}
		return LocalIdentity{Name: name}, nil
	}
	if _, ok := cfg.Apps[selector]; !ok {
		return LocalIdentity{}, fmt.Errorf("%s app %q is not configured", ConfigFile, selector)
	}
	return LocalIdentity{Name: selector}, nil
}

func ResolveAllLocalIdentities(cfg Config) ([]LocalIdentity, error) {
	if len(cfg.Apps) == 0 {
		identity, err := ResolveLocalIdentity(cfg, "")
		if err != nil { return nil, err }
		return []LocalIdentity{identity}, nil
	}
	identities := make([]LocalIdentity, 0, len(cfg.Apps))
	for _, name := range orderedAppNames(cfg) {
		identities = append(identities, LocalIdentity{Name: name})
	}
	return identities, nil
}
```

- [ ] **Step 4: Run project tests**

Run: `go test ./internal/project -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the resolver**

```bash
git add internal/project/config.go internal/project/config_test.go
git commit -m "feat: resolve local lifecycle identities"
```

### Task 2: Define Safe Local State and Lock Ownership

**Files:**
- Create: `internal/locallifecycle/types.go`
- Create: `internal/locallifecycle/state.go`
- Create: `internal/locallifecycle/lock_unix.go`
- Create: `internal/locallifecycle/state_test.go`

**Interfaces:**
- Produces: `type Target struct { ProjectPath, Name, Command, Dir string }`
- Produces: `type Identity struct { ProjectPath, Name string }`
- Produces: `type Options struct { CacheDir, Executable string; SupervisorArgs []string; StartupTimeout, StopTimeout, GracePeriod time.Duration }`
- Produces: `func DefaultOptions() (Options, error)`
- Produces internal `pathsFor(cacheDir string, identity Identity) (statePaths, error)` and lock helpers used by later tasks.

- [ ] **Step 1: Write failing path, state, and lock tests**

Cover canonical-path isolation, selector path traversal, atomic JSON round-trip, and exclusive lock ownership:

```go
func TestPathsForIsolatesProjectsAndUnsafeSelectors(t *testing.T) {
	cache := t.TempDir()
	a, err := pathsFor(cache, Identity{ProjectPath: "/work/a/nova.yaml", Name: "../api"})
	if err != nil { t.Fatal(err) }
	b, err := pathsFor(cache, Identity{ProjectPath: "/work/b/nova.yaml", Name: "../api"})
	if err != nil { t.Fatal(err) }
	if a.Dir == b.Dir || !strings.HasPrefix(a.Dir, cache) || strings.Contains(strings.TrimPrefix(a.Dir, cache), "..") {
		t.Fatalf("unsafe paths: a=%q b=%q", a.Dir, b.Dir)
	}
}

func TestStateLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	first, owned, err := tryLock(path)
	if err != nil || !owned { t.Fatalf("first lock: owned=%v err=%v", owned, err) }
	defer first.Close()
	second, owned, err := tryLock(path)
	if err != nil || owned { t.Fatalf("second lock: owned=%v err=%v", owned, err) }
	_ = second.Close()
}
```

- [ ] **Step 2: Verify state tests fail**

Run: `go test ./internal/locallifecycle -run 'Test(PathsFor|State)' -count=1`

Expected: FAIL because the package APIs do not exist.

- [ ] **Step 3: Implement types, hashed layout, atomic state, and Unix locks**

Use SHA-256 prefixes for both canonical project path and app name, create directories with `0700`, payload/state files with `0600`, and `syscall.Flock(fd, LOCK_EX|LOCK_NB)` for ownership. Define state metadata explicitly:

```go
type stateRecord struct {
	ProjectPath   string `json:"project_path"`
	Name          string `json:"name"`
	SupervisorPID int    `json:"supervisor_pid"`
	ChildPID      int    `json:"child_pid"`
	Nonce         string `json:"nonce"`
	StartedAt     string `json:"started_at"`
}

type statePaths struct {
	Dir, Lock, State, Payload, Log string
}
```

`writeJSONAtomic` must create a temporary file in the same directory, `Sync`, `Close`, `Chmod(0600)`, and rename it over the target. `DefaultOptions` must use `os.UserCacheDir()` and `os.Executable()`, set `SupervisorArgs` to `[]string{"__local-supervisor"}`, and set startup, stop, and grace timeouts to finite positive values. Tests replace `SupervisorArgs` with Go helper-process arguments without changing the production protocol.

- [ ] **Step 4: Run state tests under the race detector**

Run: `go test -race ./internal/locallifecycle -run 'Test(PathsFor|State)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit state ownership**

```bash
git add internal/locallifecycle/types.go internal/locallifecycle/state.go internal/locallifecycle/lock_unix.go internal/locallifecycle/state_test.go
git commit -m "feat: add local lifecycle state ownership"
```

### Task 3: Start a Detached Supervisor and Acknowledge Readiness

**Files:**
- Create: `internal/locallifecycle/process_unix.go`
- Create: `internal/locallifecycle/supervisor.go`
- Create: `internal/locallifecycle/lifecycle.go`
- Create: `internal/locallifecycle/lifecycle_test.go`

**Interfaces:**
- Produces: `type StartResult struct { Name, LogPath string; AlreadyRunning bool }`
- Produces: `func Start(ctx context.Context, targets []Target, opts Options) ([]StartResult, error)`
- Produces: `func RunSupervisor(payloadPath string, lockFile, readyFile *os.File) error`
- Consumes: state paths and locks from Task 2.

- [ ] **Step 1: Write a failing real-process start test**

Use the Go test helper-process pattern so the production start path still forks an executable. The test command writes its PID, prints to stdout, and waits for TERM:

```go
func TestStartReturnsPromptlyAndLeavesCommandRunning(t *testing.T) {
	opts := testOptions(t)
	project := filepath.Join(t.TempDir(), "nova.yaml")
	dir := filepath.Dir(project)
	pidFile := filepath.Join(dir, "child.pid")
	started := time.Now()
	results, err := Start(context.Background(), []Target{{
		ProjectPath: project,
		Name: "api",
		Dir: dir,
		Command: fmt.Sprintf("printf $$ > %s; printf ready; trap 'exit 0' TERM; while :; do sleep 1; done", shellQuote(pidFile)),
	}}, opts)
	if err != nil { t.Fatal(err) }
	if time.Since(started) >= opts.StartupTimeout { t.Fatal("start waited for application exit") }
	if len(results) != 1 || results[0].LogPath == "" { t.Fatalf("results=%+v", results) }
	awaitFileContains(t, results[0].LogPath, "ready")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func awaitFileContains(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(raw), want) { return }
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never contained %q", path, want)
}

func testOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		CacheDir: t.TempDir(), Executable: os.Args[0],
		SupervisorArgs: []string{"-test.run=TestSupervisorProcess", "--"},
		StartupTimeout: 2 * time.Second, StopTimeout: 2 * time.Second, GracePeriod: 100 * time.Millisecond,
	}
}

func TestSupervisorProcess(t *testing.T) {
	if os.Getenv("NOVA_LOCAL_SUPERVISOR") != "1" { return }
	payload := os.Getenv("NOVA_LOCAL_PAYLOAD")
	lockFile := os.NewFile(3, "lock")
	readyFile := os.NewFile(4, "ready")
	if err := RunSupervisor(payload, lockFile, readyFile); err != nil { os.Exit(1) }
	os.Exit(0)
}
```

Add tests for invalid targets, immediate shell start failure, inherited working directory/environment, and a second `Start` returning `AlreadyRunning: true` without changing the child PID.

- [ ] **Step 2: Verify detached-start tests fail**

Run: `go test ./internal/locallifecycle -run 'TestStart' -count=1 -v`

Expected: FAIL because `Start` and `RunSupervisor` do not exist.

- [ ] **Step 3: Implement the inherited-lock and readiness protocol**

The parent must acquire the app lock, write a cryptographically random 128-bit hex nonce payload, open `output.log` in append mode, create an `os.Pipe`, and start `exec.Command(opts.Executable, opts.SupervisorArgs...)` with `NOVA_LOCAL_SUPERVISOR=1` and `NOVA_LOCAL_PAYLOAD=<payload-path>`. Pass the lock and readiness writer through `Cmd.ExtraFiles`; set the supervisor command to a new session in `prepareSupervisorProcess`.

Represent the payload without secrets beyond the already-inherited environment:

```go
type supervisorPayload struct {
	ProjectPath string        `json:"project_path"`
	Name        string        `json:"name"`
	Command     string        `json:"command"`
	Dir         string        `json:"dir"`
	StatePath   string        `json:"state_path"`
	Nonce       string        `json:"nonce"`
	GracePeriod time.Duration `json:"grace_period"`
}

type readiness struct {
	PID      int    `json:"pid,omitempty"`
	ChildPID int    `json:"child_pid,omitempty"`
	Error    string `json:"error,omitempty"`
}
```

Wait for one JSON readiness object in a goroutine selected against `ctx.Done()` and `StartupTimeout`. On success call `cmd.Process.Release()` and return. On timeout or error, terminate the supervisor, wait boundedly, remove payload/state, release the lock, and return an app-labelled error.

The supervisor reads and removes the payload, validates every path/name against the payload and inherited lock, starts `sh -lc` with `Dir` and a new application process group, atomically writes `state.json`, sends readiness, then waits for child exit or a termination signal. It appends child exit information to the inherited log and always reaps the child and removes active state before releasing the lock.

- [ ] **Step 4: Run detached-start tests repeatedly and with race detection**

Run: `go test ./internal/locallifecycle -run 'TestStart' -count=5 -v`

Run: `go test -race ./internal/locallifecycle -run 'TestStart' -count=1`

Expected: PASS with no leaked helper process.

- [ ] **Step 5: Commit detached start**

```bash
git add internal/locallifecycle/process_unix.go internal/locallifecycle/supervisor.go internal/locallifecycle/lifecycle.go internal/locallifecycle/lifecycle_test.go
git commit -m "feat: start detached local supervisors"
```

### Task 4: Add Bounded Stop, Restart, and Multi-App Rollback

**Files:**
- Modify: `internal/locallifecycle/supervisor.go`
- Modify: `internal/locallifecycle/lifecycle.go`
- Modify: `internal/locallifecycle/lifecycle_test.go`

**Interfaces:**
- Produces: `type StopResult struct { Name string; AlreadyStopped bool }`
- Produces: `func Stop(ctx context.Context, identities []Identity, opts Options) ([]StopResult, error)`
- Produces: `func Restart(ctx context.Context, targets []Target, opts Options) ([]StartResult, error)`
- Consumes: `Start`, state ownership, and supervisor metadata from Tasks 2-3.

- [ ] **Step 1: Write failing stop/restart/rollback tests**

Add real shell tests that assert stop removes a trapped process and its background descendant, a repeated stop is idempotent, restart changes the child PID, an INT/TERM-ignoring child is killed within `StopTimeout`, stale unlocked state never signals its recorded PID, and partial multi-start failure rolls back only newly started apps:

```go
func TestRestartReplacesChildAndReturnsPromptly(t *testing.T) {
	opts := testOptions(t)
	dir := t.TempDir()
	target := Target{ProjectPath: filepath.Join(dir, "nova.yaml"), Name: "api", Dir: dir, Command: "trap 'exit 0' TERM; while :; do sleep 1; done"}
	first, err := Start(context.Background(), []Target{target}, opts)
	if err != nil { t.Fatal(err) }
	statePath, _ := pathsFor(opts.CacheDir, Identity{ProjectPath: target.ProjectPath, Name: target.Name})
	oldState, err := readState(statePath.State)
	if err != nil { t.Fatal(err) }
	second, err := Restart(context.Background(), []Target{target}, opts)
	if err != nil { t.Fatal(err) }
	newState, err := readState(statePath.State)
	if err != nil { t.Fatal(err) }
	if oldState.ChildPID == newState.ChildPID { t.Fatalf("child PID did not change: %d", oldState.ChildPID) }
	if len(first) != 1 || len(second) != 1 { t.Fatal("missing start result") }
}
```

- [ ] **Step 2: Verify lifecycle tests fail for missing behavior**

Run: `go test ./internal/locallifecycle -run 'Test(Stop|Restart|StartAll)' -count=1 -v`

Expected: FAIL because stop/restart and rollback are missing.

- [ ] **Step 3: Implement safe, bounded shutdown and restart**

On stop, attempt the lock. Owning it means no supervisor is active: remove stale payload/state and return `AlreadyStopped`. Failure to acquire means read and validate `state.json` against canonical project/name before signaling the supervisor PID. Send SIGTERM, poll the lock with a short ticker until it becomes acquirable, and stop at `StopTimeout`; on timeout send SIGKILL only after re-reading matching locked state, then wait once more for a short bounded interval. Return an error instead of claiming success if ownership cannot be confirmed released.

Supervisor termination must call the same process-group sequence as foreground local run: TERM, grace wait, KILL, reap. `Restart` prevalidates every target, stops all identities, then calls `Start`. Update `Start` to record which locks were newly acquired and roll only those back in reverse order after a later start failure.

- [ ] **Step 4: Run lifecycle tests repeatedly and under race detection**

Run: `go test ./internal/locallifecycle -count=5`

Run: `go test -race ./internal/locallifecycle -count=1`

Expected: PASS; every test cleanup confirms no child or descendant survives.

- [ ] **Step 5: Commit complete local lifecycle behavior**

```bash
git add internal/locallifecycle/supervisor.go internal/locallifecycle/lifecycle.go internal/locallifecycle/lifecycle_test.go
git commit -m "feat: stop and restart local supervisors"
```

### Task 5: Route CLI Lifecycle Commands Locally by Default

**Files:**
- Modify: `cmd/nova/main.go`
- Create: `cmd/nova/lifecycle_test.go`
- Modify: `cmd/nova/run_test.go`

**Interfaces:**
- Produces internal `type lifecycleArgs struct { Selector string; Remote bool }`
- Produces internal `func parseLifecycleArgs(args []string) (lifecycleArgs, error)`
- Produces internal `type lifecycleStreams struct { Stdout, Stderr io.Writer }`
- Produces internal `func runConfiguredLocalLifecycle(ctx context.Context, action, dir string, parsed lifecycleArgs, streams lifecycleStreams) error`
- Produces internal `type remoteLifecycleClient interface { Start(context.Context, string) error; Stop(context.Context, string) error; Restart(context.Context, string) error }`
- Produces internal `func runConfiguredRemoteLifecycle(ctx context.Context, cli remoteLifecycleClient, action, selector string) error`
- Consumes: project identity/run resolvers and `locallifecycle.Start`, `Stop`, `Restart`.

- [ ] **Step 1: Write failing parser and routing tests**

Test zero/one selector, `all`, `--remote` before/after selector, duplicate flags, unknown flags, and multiple selectors. Add local-only config tests with all endpoint/token variables blank and assert start/stop/restart do not call `autoBootstrapRuntimeConfig`. Add a `fakeRemoteLifecycleClient` implementing the exact three-method interface and recording `action+":"+app` strings. Assert default local parsing never constructs or calls it, while `--remote` reaches `runConfiguredRemoteLifecycle` and records the expected configured remote app.

```go
func TestParseLifecycleArgsAcceptsRemoteAnywhere(t *testing.T) {
	for _, args := range [][]string{{"--remote", "api"}, {"api", "--remote"}} {
		got, err := parseLifecycleArgs(args)
		if err != nil { t.Fatal(err) }
		if !got.Remote || got.Selector != "api" { t.Fatalf("got %+v", got) }
	}
}

func TestAutoBootstrapRuntimeConfigSkipsDefaultLocalLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, action := range []string{"start", "stop", "restart"} {
		if err := autoBootstrapRuntimeConfig(action); err != nil { t.Fatalf("%s: %v", action, err) }
	}
}
```

- [ ] **Step 2: Verify CLI tests fail**

Run: `go test ./cmd/nova -run 'Test(ParseLifecycle|AutoBootstrapRuntimeConfigSkipsDefaultLocalLifecycle|ConfiguredLocalLifecycle)' -count=1`

Expected: FAIL because lifecycle parsing/routing does not exist and bootstrap still treats lifecycle commands as remote.

- [ ] **Step 3: Implement pre-bootstrap routing and private supervisor entry**

Before `autoBootstrapRuntimeConfig`, detect the private `__local-supervisor` entry and lifecycle commands. Parse lifecycle args once. For local mode call the project local configuration and lifecycle package. For remote mode explicitly bootstrap runtime config, construct the existing client, and run the existing Agent loops.

Use the resolver distinction exactly. Implement the conversion loops with `ProjectPath: path`, `Dir: filepath.Dir(path)`, and the resolved local name/command; return resolution errors before calling the lifecycle package:

```go
switch action {
case "start", "restart":
	var runs []project.RunTarget
	if parsed.Selector == "all" {
		runs, err = project.ResolveAllRuns(cfg)
	} else {
		var run project.RunTarget
		run, err = project.ResolveRun(cfg, parsed.Selector)
		runs = []project.RunTarget{run}
	}
case "stop":
	var identities []project.LocalIdentity
	if parsed.Selector == "all" {
		identities, err = project.ResolveAllLocalIdentities(cfg)
	} else {
		var identity project.LocalIdentity
		identity, err = project.ResolveLocalIdentity(cfg, parsed.Selector)
		identities = []project.LocalIdentity{identity}
	}
}
```

The private supervisor entry obtains payload, lock FD, and readiness FD only from parent-set environment/extra files; reject direct invocation if any are absent. Update usage text to show local default and `--remote` syntax. Print local results with app and log path; retain current remote success text unchanged.

- [ ] **Step 4: Run CLI and foreground-run regression tests**

Run: `go test ./cmd/nova ./internal/project ./internal/localrun ./internal/locallifecycle -count=1`

Run: `go test ./internal/localrun -count=5`

Expected: PASS, including existing foreground signal cleanup.

- [ ] **Step 5: Commit CLI routing**

```bash
git add cmd/nova/main.go cmd/nova/lifecycle_test.go cmd/nova/run_test.go
git commit -m "feat: default lifecycle commands to local runs"
```

### Task 6: Document the Breaking Default and Prepare v0.1.14

**Files:**
- Modify: `README.md`
- Modify: `docs/nova-run-spec.md`
- Modify: `CHANGELOG.md`
- Modify: `VERSION`

**Interfaces:**
- Documents: local lifecycle commands, log discovery, idempotency, `all`, and explicit remote syntax.
- Produces: version metadata `0.1.14` and release notes for tag `v0.1.14`.

- [ ] **Step 1: Update README command examples and migration warning**

Replace remote lifecycle examples in both `README.md` and `docs/nova-run-spec.md` with explicit flags:

```text
nova start --remote [app|all]
nova stop --remote [app|all]
nova restart --remote [app|all]
```

Add a local section showing `nova start`, `nova stop`, and `nova restart`, explaining that start detaches and prints the cache log path, repeated start/stop are idempotent, and `nova run` remains foreground. Add a prominent migration note: scripts written before `v0.1.14` that intend to manage deployed services must add `--remote`.

- [ ] **Step 2: Update changelog and version metadata**

Move relevant entries into a new `## [0.1.14] - 2026-08-25` section with Added/Changed/Fixed headings. The Changed entry must call out the breaking default. Replace `VERSION` contents with exactly:

```text
0.1.14
```

- [ ] **Step 3: Verify docs and version references**

Run: `rg -n 'nova (start|stop|restart)' README.md docs/nova-run-spec.md`

Run: `test "$(tr -d '\n' < VERSION)" = "0.1.14"`

Expected: every deployed-service example uses `--remote`, local examples do not, and VERSION matches the tag without the `v` prefix.

- [ ] **Step 4: Commit release documentation**

```bash
git add README.md docs/nova-run-spec.md CHANGELOG.md VERSION
git commit -m "docs: prepare v0.1.14 local lifecycle release"
```

### Task 7: Verify, Review, and Publish the Release

**Files:**
- Modify only if verification or review finds a defect; every fix requires a failing regression test first.

**Interfaces:**
- Consumes: complete implementation from Tasks 1-6.
- Produces: pushed `main`, annotated tag `v0.1.14`, successful release workflow, and verified downloadable assets.

- [ ] **Step 1: Format and run static checks**

Run: `gofmt -w cmd/nova/main.go cmd/nova/lifecycle_test.go cmd/nova/run_test.go internal/project/config.go internal/project/config_test.go internal/locallifecycle/*.go`

Run: `git diff --check`

Run: `go vet ./...`

Expected: no formatting errors, whitespace errors, or vet findings.

- [ ] **Step 2: Run the complete test suite with race detection**

Run: `go test ./... -count=1`

Run: `go test -race ./... -count=1`

Expected: PASS with no races, hangs, leaked-process failures, warnings, or skipped lifecycle assertions.

- [ ] **Step 3: Cross-compile every release artifact locally**

Run these separately so any failing target is obvious:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o /tmp/nova-linux-amd64 ./cmd/nova
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o /tmp/nova-linux-arm64 ./cmd/nova
GOOS=darwin GOARCH=amd64 go build -trimpath -o /tmp/nova-darwin-amd64 ./cmd/nova
GOOS=darwin GOARCH=arm64 go build -trimpath -o /tmp/nova-darwin-arm64 ./cmd/nova
```

Expected: all four commands succeed. Remove only these four explicit `/tmp/nova-*` files after inspection.

- [ ] **Step 4: Perform completion verification and code review**

Read `superpowers:verification-before-completion`, then `superpowers:requesting-code-review`. Compare the final diff against every design requirement. If review finds an issue, add a failing test, fix it, and repeat Steps 1-4.

- [ ] **Step 5: Confirm clean release state and push main**

Run: `git status --short`

Run: `git log --oneline v0.1.13..HEAD`

Expected: clean worktree and only the intended design, implementation, test, and documentation commits.

Run: `git push origin main`

Expected: push succeeds and GitHub `main` points at the verified commit.

- [ ] **Step 6: Create and push the release tag**

Run: `git tag -a v0.1.14 -m "nova-run v0.1.14"`

Run: `git push origin v0.1.14`

Expected: tag push triggers `.github/workflows/release.yml`.

- [ ] **Step 7: Wait for and verify GitHub release assets**

Run: `gh run list --workflow Release --limit 5`

Wait using bounded `gh run watch <run-id> --exit-status` intervals while reporting progress at least once per minute.

Run: `gh release view v0.1.14 --json tagName,isLatest,assets,url`

Expected assets:

```text
nova-linux-amd64
nova-linux-arm64
nova-darwin-amd64
nova-darwin-arm64
SHA256SUMS.txt
```

Download assets to a new `mktemp -d` directory, run `sha256sum -c SHA256SUMS.txt` on Linux or `shasum -a 256 -c SHA256SUMS.txt` on macOS, then report the release URL and verification result. The temporary directory may be removed after checks pass.
