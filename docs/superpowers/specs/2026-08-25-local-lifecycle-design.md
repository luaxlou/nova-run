# Nova Run Local Lifecycle Design

**Date:** 2026-08-25

## Goal

Make `nova start`, `nova stop`, and `nova restart` manage the current project's local `run` commands by default. Local starts must detach from the invoking terminal and return after startup is confirmed. The existing Nova Agent lifecycle operations remain available through an explicit `--remote` flag.

`nova run` remains the foreground development command. This preserves its attached stdin/stdout behavior and its existing `Ctrl+C` cleanup contract.

## CLI Contract

The lifecycle commands accept zero or one app selector plus an optional `--remote` flag:

```text
nova start [app|all] [--remote]
nova stop [app|all] [--remote]
nova restart [app|all] [--remote]
```

`--remote` is accepted before or after the selector. Unknown flags, duplicate selectors, and duplicate `--remote` flags are errors.

Without `--remote`, the commands load the nearest `nova.yaml` through the local-run configuration path and resolve `run` commands using the same default selector and `all` ordering as `nova run`. They do not bootstrap remote configuration, read an Agent token, or create a remote client.

With `--remote`, the commands retain their current behavior: load deploy targets from `nova.yaml`, use the configured Nova Agent, and print the existing remote success messages.

Other commands are unchanged. In particular, `status` and `logs` remain remote-only in this change.

## Local Supervisor Architecture

A detached Nova subprocess supervises each selected local app. This is preferable to releasing a raw shell child because the supervisor can own the process group, reap descendants, maintain an exclusive state lock, and apply the same bounded TERM/KILL cleanup policy after the original CLI exits.

The public command process performs these steps for each selected app:

1. Resolve all requested local commands before starting any process.
2. Derive a stable state directory from the canonical `nova.yaml` path and local selector name.
3. Check the app state lock. If it is held, report that the app is already running without starting a duplicate.
4. Start the current Nova executable in a private supervisor mode with stdin disconnected and stdout/stderr appended to the app's local log.
5. Wait for a bounded readiness acknowledgement from the supervisor.
6. Return success immediately after the supervisor has started and recorded its state. A failed or timed-out readiness check is an error and must not leave an untracked child.

The private supervisor loads the already-resolved command from a parent-owned startup payload, creates a new process group/session, starts `sh -lc <run-command>` in the `nova.yaml` directory, holds the state lock for its entire lifetime, records its PID and command metadata, forwards termination to the complete child process group, and removes transient state on exit. The command is not passed in process-list arguments.

The private supervisor entry point is deliberately undocumented and rejected when its required parent-generated payload is missing. It is an implementation detail, not a supported CLI API.

## State and Logs

Local lifecycle data lives below `os.UserCacheDir()` so using Nova does not dirty the project repository. The layout is conceptually:

```text
<user-cache>/nova/run/<project-key>/<app-key>/
  lock
  state.json
  output.log
```

The project key is a hash of the canonical `nova.yaml` path. Human-readable canonical project and app names remain in `state.json` for diagnostics. App keys are escaped or hashed so selectors cannot traverse directories.

The lock, rather than the PID file alone, is the source of truth for whether a Nova supervisor owns the state. This prevents a stale PID from causing an unrelated reused PID to be killed. State writes use a temporary file plus rename so readers never observe partial JSON.

Logs append across starts. `nova start` prints the log path to make detached output discoverable. Log rotation and local `nova logs` support are outside this change.

## Start, Stop, and Restart Semantics

### Start

- Validate every selected `run` command before starting any supervisor.
- Start selected apps in declaration order.
- Treat an already-running app as an idempotent success and identify it as already running.
- If a later app fails to start during `all`, stop only the supervisors started by this invocation; pre-existing apps remain running.
- The foreground command has a bounded startup wait and never waits for the application to exit.

Readiness means the supervisor owns its state lock and successfully started the shell process. It does not imply application-level health.

### Stop

- Resolve the requested app identities from `nova.yaml`; a `run` command is not required merely to stop an existing supervisor.
- If no supervisor owns the state lock, remove stale metadata and report the app as already stopped.
- Otherwise signal the recorded supervisor, wait a bounded grace period, then force termination if required.
- The supervisor is responsible for terminating and reaping the complete application process group.
- Never signal a PID unless the matching state lock is currently owned and the metadata identifies the same canonical project and app.

### Restart

- Resolve and validate all requested `run` commands first.
- Stop each requested local supervisor using the local stop semantics.
- Start fresh supervisors only after all stops complete.
- Return an error if cleanup cannot be confirmed; do not layer a second process over an uncertain existing one.

## Process and Signal Handling

Unix is the supported process-management target because the current local runner already depends on Unix process groups. The detached supervisor starts in its own session. Its application shell starts in a distinct process group so shutdown can target all descendants without terminating the supervisor before it has reaped them and cleared state.

On SIGINT, SIGTERM, or an explicit local stop request, the supervisor forwards TERM to the application process group, waits up to the configured grace period, sends KILL to survivors, reaps the shell, clears state, releases the lock, and exits. All waits have explicit deadlines.

Unexpected application exit causes the supervisor to record the exit in the log, clear its active state, and exit. This change does not add automatic restart.

## Error Handling

Errors include the local selector and the relevant state or log path where useful, but never print the resolved environment or remote token. Startup distinguishes configuration errors, already-running state, supervisor bootstrap failure, and application start failure.

Stop and restart use bounded waits so stale or uncooperative processes cannot hang the CLI. Partial `all` failures report the primary error plus any rollback cleanup error.

## Code Organization

- `cmd/nova/main.go` routes local lifecycle commands before remote bootstrap, parses `--remote`, and retains the existing remote adapters.
- Focused command tests under `cmd/nova` verify routing, argument handling, and remote-bootstrap bypass.
- A new `internal/locallifecycle` package owns cache paths, state metadata, locking, supervisor launch/readiness, and start/stop/restart orchestration.
- Unix-specific locking, session, and signal operations live in `_unix.go` files.
- Existing `internal/localrun` remains responsible for foreground `nova run`; shared low-level process helpers may be extracted only when doing so reduces duplication without changing foreground behavior.

## Test Strategy

Implementation follows red-green-refactor. Tests use real short-lived shell processes where process behavior matters and injected adapters only at CLI routing boundaries.

Coverage includes:

- lifecycle flag parsing with `--remote` before and after selectors;
- default local routing without endpoint/token configuration;
- explicit remote routing retaining existing target resolution and client calls;
- detached start returning promptly while the application remains alive;
- application output reaching the advertised log;
- idempotent repeated start;
- stop of running, stopped, and stale-state apps;
- restart replacing the process and returning promptly;
- `all` ordering, preflight validation, and rollback after partial startup;
- process-group cleanup including background descendants;
- uncooperative child escalation with bounded completion;
- canonical project isolation and selectors that contain unsafe path characters;
- no regression in foreground `nova run` signal and exit-code behavior;
- `go test -race ./...`, formatting, vetting, and cross-compilation for the release targets.

## Documentation and Compatibility

README usage and examples will state that lifecycle commands are local by default and show `--remote` for deployed services. Existing scripts that call `nova start`, `nova stop`, or `nova restart` for remote services must add `--remote`; this is an intentional breaking CLI-default change and will be called out prominently in the changelog and release notes.

No `nova.yaml` schema changes are required. The same `run` field powers foreground `nova run` and detached local lifecycle commands, while `service.command` remains remote-only.

## Release

After implementation, tests, and review pass:

1. Update `CHANGELOG.md` with the breaking default change and the new local supervisor behavior.
2. Update the repository version metadata consistently with the existing release convention.
3. Commit the release-ready changes and push `main`.
4. Create and push tag `v0.1.14`, the next patch tag after `v0.1.13`.
5. Wait for `.github/workflows/release.yml` to build Linux and macOS artifacts for amd64 and arm64.
6. Confirm the GitHub release exists, all four binaries and `SHA256SUMS.txt` are attached, and the latest installer resolves the new release.

The release is not complete merely when the tag is pushed; workflow and downloadable-asset verification are required.

## Out of Scope

- Changing `nova run` from foreground to background.
- Local implementations of `status` or `logs`.
- Application health checks or readiness probes.
- Automatic restart, watch mode, dependency ordering, or log rotation.
- Windows process management.
- A machine-wide Nova daemon.
