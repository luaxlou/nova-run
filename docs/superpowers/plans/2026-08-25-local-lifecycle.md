# Nova Run Stateless Local Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Execute configured local `start` and `stop` commands statelessly, define restart as stop+start, make run an alias for restart, and require `--remote` for Nova Agent lifecycle operations.

**Architecture:** Replace foreground local supervision with project-level lifecycle resolution and a small synchronous shell-command executor. Route all lifecycle commands before Agent bootstrap and select either the local executor or the unchanged remote client from a shared `--remote` parser.

**Tech Stack:** Go 1.24.5, `sh -lc`, existing YAML/project packages, GitHub Actions release workflow.

**Spec:** `docs/superpowers/specs/2026-08-25-local-lifecycle-design.md`

## Global Constraints

- Nova creates no supervisor, daemon, PID/lock file, cache record, or process registry.
- Config contains `start` and `stop`; there is no configured `restart`.
- Restart and run both execute stop then start, stopping on the first failure.
- A local start command must detach itself if the application should outlive it.
- Local commands bypass Agent configuration; `--remote` retains existing Agent behavior.
- Resolve and validate every selected target before executing any command.
- Preserve child exit codes and existing deploy/status/log/list/remove behavior.
- Publish and verify `v0.1.14` after all checks pass.

---

### Task 1: Replace Run Configuration with Stateless Lifecycle Resolution

**Files:**
- Modify: `internal/project/config.go`
- Modify: `internal/project/config_test.go`

**Interfaces:**
- Produces: `type LifecycleAction string` with `ActionStart` and `ActionStop`.
- Produces: `type LifecycleTarget struct { Name, Start, Stop string }`.
- Produces: `func LoadForLifecycle(dir string) (Config, string, error)`.
- Produces: `func ResolveLifecycle(cfg Config, selector string, actions ...LifecycleAction) (LifecycleTarget, error)`.
- Produces: `func ResolveAllLifecycles(cfg Config, actions ...LifecycleAction) ([]LifecycleTarget, error)`.

- [ ] **Step 1: Write failing configuration tests**

Add tests for root-only lifecycle config, named/inherited commands, first-declared default, YAML ordering, action-specific validation, both-action preflight, lifecycle-only load, and rejection of the old `run` key:

```go
func TestResolveLifecycleRestartRequiresStartAndStop(t *testing.T) {
	cfg := Config{Start: "printf start", Stop: "printf stop"}
	got, err := ResolveLifecycle(cfg, "", ActionStop, ActionStart)
	if err != nil { t.Fatal(err) }
	if got.Name != "default" || got.Start != "printf start" || got.Stop != "printf stop" {
		t.Fatalf("target = %+v", got)
	}
}

func TestResolveAllLifecyclesPreflightsEveryApp(t *testing.T) {
	cfg := Config{Apps: map[string]App{"api": {Start: "start", Stop: "stop"}, "web": {Stop: "stop"}}, AppOrder: []string{"api", "web"}}
	_, err := ResolveAllLifecycles(cfg, ActionStop, ActionStart)
	if err == nil || !strings.Contains(err.Error(), "apps.web start is required") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/project -run 'Test(ResolveLifecycle|ResolveAllLifecycles|LoadForLifecycle)' -count=1`

Expected: FAIL because lifecycle fields and APIs do not exist.

- [ ] **Step 3: Implement lifecycle schema and resolvers**

Replace `Run string` with fields on both config structs:

```go
Start string `json:"start,omitempty" yaml:"start,omitempty"`
Stop  string `json:"stop,omitempty" yaml:"stop,omitempty"`
```

Use one validator for NUL/newline/carriage-return. `LoadForLifecycle` parses YAML, rejects any top-level or app `run` node with `nova.yaml run is no longer supported; configure start and stop`, and does not call deploy validation. Resolve root or the first/named app, inherit root values with `firstNonEmpty`, then validate only requested actions. `ResolveAllLifecycles` builds the complete slice before returning it and preserves `AppOrder`.

- [ ] **Step 4: Run project tests and verify GREEN**

Run: `go test ./internal/project -count=1`

Expected: PASS after updating obsolete run-resolution tests to the new lifecycle contract.

- [ ] **Step 5: Commit**

```bash
git add internal/project/config.go internal/project/config_test.go
git commit -m "feat: resolve stateless local lifecycle commands"
```

### Task 2: Execute Local Command Sequences and Preserve Exit Codes

**Files:**
- Create: `internal/localcommand/executor.go`
- Create: `internal/localcommand/executor_test.go`
- Delete: `internal/localrun/runner.go`
- Delete: `internal/localrun/process_unix.go`
- Delete: `internal/localrun/runner_test.go`

**Interfaces:**
- Produces: `type Command struct { Target, Action, ShellCommand string }`.
- Produces: `type Streams struct { Stdin io.Reader; Stdout, Stderr io.Writer }`.
- Produces: `type ExitError struct { Target, Action, Command string; Code int; Err error }` with `Error`, `Unwrap`, and `ExitCode`.
- Produces: `func Run(ctx context.Context, commands []Command, dir string, streams Streams) error`.

- [ ] **Step 1: Write failing executor tests**

Use real shell commands to cover order, working directory, environment/stdin/stdout, stop-on-failure, and exit-code propagation:

```go
func TestRunExecutesCommandsInOrderAndStopsOnFailure(t *testing.T) {
	var stdout bytes.Buffer
	err := Run(context.Background(), []Command{
		{Target: "api", Action: "stop", ShellCommand: "printf stop; exit 7"},
		{Target: "api", Action: "start", ShellCommand: "printf start"},
	}, t.TempDir(), Streams{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: io.Discard})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 { t.Fatalf("err = %v", err) }
	if stdout.String() != "stop" { t.Fatalf("stdout = %q", stdout.String()) }
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/localcommand -count=1`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement the minimal synchronous executor**

Validate non-empty commands, directory, and streams before execution. For each command print `[target] $ <action>: <command>` only when more than one target/action is present, then use:

```go
cmd := exec.CommandContext(ctx, "sh", "-lc", command.ShellCommand)
cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = dir, streams.Stdin, streams.Stdout, streams.Stderr
err := cmd.Run()
```

Map `*exec.ExitError.ExitCode()` into `ExitError`; map start/context errors to code 1. Delete `internal/localrun` only after the replacement tests pass and no imports remain.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `go test ./internal/localcommand -count=1`

Run: `go test -race ./internal/localcommand -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/localcommand internal/localrun
git commit -m "feat: execute stateless local command sequences"
```

### Task 3: Route Start, Stop, Restart, and Run Locally by Default

**Files:**
- Modify: `cmd/nova/main.go`
- Replace: `cmd/nova/run_test.go`
- Create: `cmd/nova/lifecycle_test.go`

**Interfaces:**
- Produces: `type lifecycleArgs struct { Selector string; Remote bool }`.
- Produces: `func parseLifecycleArgs(args []string) (lifecycleArgs, error)`.
- Produces: `func runConfiguredLocalLifecycle(ctx context.Context, dir, action string, parsed lifecycleArgs, streams localcommand.Streams) error`.
- Produces: `type remoteLifecycleClient interface` with existing Start/Stop/Restart signatures.
- Produces: `func runConfiguredRemoteLifecycle(ctx context.Context, cli remoteLifecycleClient, action, selector string) error`.

- [ ] **Step 1: Write failing parser and local-routing tests**

Cover flags before/after selectors, duplicate/unknown flags, local Agent-bootstrap bypass, exact command expansion, all-target preflight, run/restart equivalence, and exit codes:

```go
func TestRunAndRestartUseTheSameLocalSequence(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("start: 'printf start >> events'\nstop: 'printf stop >> events'\n")
	if err := os.WriteFile(filepath.Join(dir, project.ConfigFile), raw, 0o644); err != nil { t.Fatal(err) }
	for _, action := range []string{"restart", "run"} {
		if err := runConfiguredLocalLifecycle(context.Background(), dir, action, lifecycleArgs{}, localcommand.Streams{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard}); err != nil { t.Fatal(err) }
	}
	content, err := os.ReadFile(filepath.Join(dir, "events"))
	if err != nil { t.Fatal(err) }
	if string(content) != "stop\nstart\nstop\nstart\n" { t.Fatalf("events = %q", content) }
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./cmd/nova -run 'Test(ParseLifecycle|RunAndRestart|ConfiguredLocalLifecycle|AutoBootstrap)' -count=1`

Expected: FAIL because parsing and local lifecycle routing are missing.

- [ ] **Step 3: Implement shared routing**

Parse one optional selector and one `--remote`. Route lifecycle commands before `autoBootstrapRuntimeConfig`. For local mode load with `LoadForLifecycle`, resolve all targets before building commands, and map actions exactly:

```go
switch action {
case "start": actions = []project.LifecycleAction{project.ActionStart}
case "stop": actions = []project.LifecycleAction{project.ActionStop}
case "restart", "run": actions = []project.LifecycleAction{project.ActionStop, project.ActionStart}
}
```

Build all `localcommand.Command` values before calling `Run`. For remote mode bootstrap explicitly, load existing deploy targets, and map `run` to `cli.Restart`. Use one `cliExitCode` helper that preserves any positive `ExitCode()`.

- [ ] **Step 4: Run CLI and regression tests**

Run: `go test ./cmd/nova ./internal/project ./internal/localcommand -count=1`

Run: `go test ./... -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nova/main.go cmd/nova/run_test.go cmd/nova/lifecycle_test.go
git commit -m "feat: default lifecycle commands to stateless local execution"
```

### Task 4: Update Documentation and Prepare v0.1.14

**Files:**
- Modify: `README.md`
- Modify: `docs/nova-run-spec.md`
- Modify: `CHANGELOG.md`
- Modify: `VERSION`

- [ ] **Step 1: Document schema, statelessness, aliases, and migration**

Replace every local `run:` example with `start:` and `stop:`. State that start commands must return by themselves, restart/run execute stop+start, Nova retains no lifecycle state, and remote examples require `--remote`. Add a migration warning for scripts and configs from versions before `v0.1.14`.

- [ ] **Step 2: Update changelog and version**

Add `## [0.1.14] - 2026-08-25` with the breaking default and run semantic changes. Set `VERSION` to exactly `0.1.14`.

- [ ] **Step 3: Verify documentation consistency**

Run: `rg -n 'run:|nova (start|stop|restart|run)' README.md docs/nova-run-spec.md`

Expected: no obsolete config field remains; remote lifecycle examples carry `--remote`.

Run: `test "$(tr -d '\n' < VERSION)" = "0.1.14"`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add README.md docs/nova-run-spec.md CHANGELOG.md VERSION
git commit -m "docs: prepare v0.1.14 stateless lifecycle release"
```

### Task 5: Verify, Review, and Publish

**Files:**
- Modify only for test-first fixes found during verification/review.

- [ ] **Step 1: Run formatting and static checks**

Run: `gofmt -w cmd/nova/*.go internal/project/*.go internal/localcommand/*.go`

Run: `git diff --check`

Run: `go vet ./...`

Expected: no findings.

- [ ] **Step 2: Run complete tests and race tests**

Run: `go test ./... -count=1`

Run: `go test -race ./... -count=1`

Expected: PASS without hangs or warnings.

- [ ] **Step 3: Cross-build release targets**

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o /tmp/nova-linux-amd64 ./cmd/nova
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o /tmp/nova-linux-arm64 ./cmd/nova
GOOS=darwin GOARCH=amd64 go build -trimpath -o /tmp/nova-darwin-amd64 ./cmd/nova
GOOS=darwin GOARCH=arm64 go build -trimpath -o /tmp/nova-darwin-arm64 ./cmd/nova
```

Expected: all four succeed.

- [ ] **Step 4: Apply completion-verification and code-review skills**

Read and follow `superpowers:verification-before-completion` and `superpowers:requesting-code-review`. Any defect gets a failing regression test before its fix.

- [ ] **Step 5: Push and publish**

Confirm `git status --short` is empty, push `main`, create annotated tag `v0.1.14`, and push the tag. Wait for the Release workflow with `gh run watch <run-id> --exit-status`.

- [ ] **Step 6: Verify release assets**

Run: `gh release view v0.1.14 --json tagName,isLatest,assets,url`

Expected: four platform binaries plus `SHA256SUMS.txt`. Download them to `mktemp -d`, verify checksums with `shasum -a 256 -c SHA256SUMS.txt` on macOS, and report the release URL.
