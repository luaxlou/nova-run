# Nova Run Stateless Local Lifecycle Design

**Date:** 2026-08-25

## Goal

Make Nova a stateless command runner for local application lifecycle commands. `nova start`, `nova stop`, `nova restart`, and `nova run` execute commands declared in the current project's `nova.yaml`; they do not create a supervisor, daemon, PID file, lock file, cache entry, or process registry.

Remote Nova Agent lifecycle operations remain available only through `--remote`.

## Configuration

Single-app projects declare local commands directly:

```yaml
start: npm run start
stop: npm run stop
```

Multi-app projects declare them per selector:

```yaml
apps:
  api:
    start: scripts/start-api.sh
    stop: scripts/stop-api.sh
  web:
    start: scripts/start-web.sh
    stop: scripts/stop-web.sh
```

Top-level `start` and `stop` are inherited by an app when that app does not override them, matching existing top-level/app inheritance patterns.

There is no configured `restart` command. Restart is always the composition `stop` followed by `start`.

The former `run: <command>` field is removed from the local lifecycle schema. This is an intentional breaking change. Projects migrate by defining both `start` and `stop`; a persistent application must make its own `start` command detach or delegate to a process manager and then return.

Deployment fields remain unchanged. `service.command` still describes the remote production process installed and controlled by Nova Agent.

## CLI Contract

```text
nova start [app|all] [--remote]
nova stop [app|all] [--remote]
nova restart [app|all] [--remote]
nova run [app|all] [--remote]
```

`--remote` is accepted before or after the optional selector. Unknown flags, repeated `--remote`, or more than one selector are errors.

Without `--remote`:

- `start` executes the resolved local `start` command.
- `stop` executes the resolved local `stop` command.
- `restart` executes the resolved local `stop`, then the resolved local `start`.
- `run` is an exact alias for local `restart`.

With `--remote`:

- `start`, `stop`, and `restart` retain their current Nova Agent behavior.
- `run --remote` is an alias for remote `restart`.

All four commands route before automatic remote configuration bootstrap. Local execution never reads an Agent endpoint/token or starts interactive `nova init`. Explicit remote execution uses the existing bootstrap and client behavior.

## Execution Semantics

Nova executes each configured local command as `sh -lc <command>` in the directory containing `nova.yaml`. The command inherits the caller's environment and standard input/output/error.

Nova waits for the configured command itself to finish and returns its exit code. It does not wait for application processes that the command correctly detached. Nova cannot make an arbitrary foreground server nonblocking without becoming a supervisor, so non-hanging local lifecycle is a configuration contract: `start` must return after starting the application.

For `restart`, Nova runs `stop` first. If `stop` fails, Nova returns that failure and does not run `start`. If `start` fails, Nova returns the start failure. `run` follows exactly the same sequence and errors.

For `all`, targets execute in YAML declaration order. Each target action completes before the next begins. A failure stops the sequence immediately; Nova does not attempt rollback because it owns no runtime state and cannot infer how user commands should be compensated.

Before executing anything, Nova resolves and validates all required commands for every selected target. This prevents a partially executed `all` caused by a missing later configuration value.

## Validation and Resolution

Project configuration adds `Start` and `Stop` fields to both the root config and app entries. Values are trimmed and rejected when empty, or when they contain NUL, newline, or carriage return characters.

Local resolution is independent of deployment validation, so a lifecycle-only `nova.yaml` does not need `app`, `build`, `artifacts`, or `service.command`. Existing deploy resolution and validation remain unchanged.

Resolution rules:

- Empty selector chooses the root lifecycle config when either root `start` or root `stop` is present.
- Otherwise, empty selector chooses the first YAML-declared app.
- A named selector resolves only from `apps.<selector>` and may inherit root `start`/`stop`.
- `all` resolves every configured app in YAML declaration order; for a root-only project it resolves one `default` target.
- `start` validates only `start`; `stop` validates only `stop`; `restart` and `run` validate both before execution.

## Error and Exit-Code Behavior

Configuration, parsing, and shell-start errors exit with code 1. A shell command that exits with a positive status propagates that status through the CLI, including `stop` failures during restart/run.

Errors identify the action and local selector without printing environment variables or remote credentials. Multi-app errors also identify the target that failed.

## Code Organization

- `internal/project/config.go` owns lifecycle fields, syntax validation, inheritance, target resolution, and declaration ordering.
- A focused `internal/localcommand` package executes validated command sequences and maps `exec.ExitError` to a stable `ExitCode()` contract.
- `cmd/nova/main.go` owns argument parsing and local/remote routing. Shared helpers keep `start`, `stop`, `restart`, and `run` behavior consistent.
- Existing `internal/localrun` is removed after its process-supervisor behavior and tests are no longer referenced.

## Test Strategy

Implementation follows red-green-refactor. Tests cover:

- root, named, inherited, default-first, and `all` resolution;
- lifecycle-only configuration without deployment fields;
- removal/rejection of the former `run` field;
- preflight of every target before `all` starts;
- `start` and `stop` executing exactly one command;
- `restart` and `run` executing stop then start;
- stop failure preventing start;
- exact child exit-code propagation;
- working directory, environment, and stdio inheritance;
- local execution without Agent configuration;
- `--remote` before and after selectors, including `run --remote` mapping to remote restart;
- no regression in deploy, status, logs, list, remove, or Agent behavior;
- full tests, race tests, vet, and four release-target cross-builds.

## Documentation and Release

README, the project spec, CLI usage, and examples will describe the new `start`/`stop` schema and the stateless behavior. Remote lifecycle examples must use `--remote`. The changelog will prominently call out both breaking changes: remote lifecycle is no longer the default, and `run` now means stateless restart rather than foreground supervision.

After verification, update version metadata to `0.1.14`, push `main`, create tag `v0.1.14`, wait for the existing GitHub Actions release workflow, and verify these downloadable assets and their checksums:

- `nova-linux-amd64`
- `nova-linux-arm64`
- `nova-darwin-amd64`
- `nova-darwin-arm64`
- `SHA256SUMS.txt`

## Out of Scope

- Supervising, daemonizing, monitoring, or automatically restarting application processes.
- PID files, state locks, cache records, status discovery, or local log storage.
- Inferring how to stop an application that has no configured `stop` command.
- Application health/readiness checks, dependency ordering, parallel `all`, or rollback.
- Changes to remote deployment artifacts or Nova Agent process management.
