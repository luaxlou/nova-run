# Nova Run Local Supervisor Design

**Date:** 2026-08-26

## Goal

Upgrade Nova's local lifecycle from stateless shell-command composition to a reliable per-application process supervisor. Local `start`, `stop`, `restart`, `run`, and `status` must return promptly, manage complete process groups, and report runtime truth without requiring Nova Agent.

This is a breaking architecture change and will be released as `v0.2.0`.

## Selected Architecture

Nova uses one detached supervisor process per running local application. There is no machine-wide Nova daemon. If no local application is running, no Nova supervisor remains active.

Alternatives rejected:

- A global Nova daemon centralizes state but adds installation, upgrade, permission, and failure-domain complexity.
- Delegating status to project-provided systemd, PM2, Docker, or custom commands keeps Nova stateless but does not provide one consistent local lifecycle model.

## Configuration Contract

Single application:

```yaml
start: go run ./cmd/api
```

Multiple applications:

```yaml
apps:
  api:
    start: go run ./cmd/api
  web:
    start: npm run dev
```

`start` is the long-running foreground application command. It must not daemonize itself. Nova starts it through `sh -lc` in the directory containing `nova.yaml`, with the caller's environment.

The local `stop:` field introduced in `v0.1.14` is removed. Stop is owned exclusively by the supervisor so there cannot be two conflicting runtime authorities. Loading local lifecycle configuration rejects any root or app `stop` key with a migration error.

Top-level `start` remains inheritable by apps. Remote `service.command` remains unchanged and independent.

## CLI Contract

```text
nova start [app|all] [--remote]
nova stop [app|all] [--remote]
nova restart [app|all] [--remote]
nova run [app|all] [--remote]
nova status [app|all] [--remote]
```

Without `--remote`, all five commands operate on local supervisors. With `--remote`, they retain Nova Agent behavior. `run` remains an exact alias for restart in both modes.

`--remote` is accepted before or after the optional selector. Unknown flags, duplicate flags, and multiple selectors are errors. Local paths route before Agent configuration bootstrap and never read Endpoint or Token.

## Target Resolution

Local start targets contain canonical `nova.yaml` path, local selector name, working directory, and resolved start command. Start, restart, and run preflight all selected start commands before changing any process.

Stop and status resolve identities without requiring a start command. This allows Nova to stop or inspect a previously started app after its start field was temporarily removed. Identity selection follows existing rules:

- Root lifecycle configuration resolves as `default`.
- Otherwise an empty selector chooses the first YAML-declared app.
- A named selector uses the local YAML selector, never a remote `app` alias.
- `all` follows YAML declaration order; a root-only project resolves once as `default`.

## State Layout and Ownership

State lives under `os.UserCacheDir()` and never dirties the project:

```text
<cache>/nova/run/<project-hash>/<app-hash>/
  lock
  state.json
  control.sock
  output.log
  startup.json
```

The project hash derives from the canonical absolute `nova.yaml` path. The app hash derives from the local selector. Human-readable project and app values remain in state metadata. Paths never include unescaped selector content.

The advisory file lock is the primary ownership signal. A PID in `state.json` is never trusted without matching lock ownership and canonical identity. The supervisor inherits the already-locked file descriptor from its starting CLI process, closing the race between checking and starting.

State writes use a temporary file, fsync, chmod `0600`, and atomic rename. Directories use `0700`. The control socket is accessible only to the current user.

## State Model

`state.json` records:

- schema version;
- canonical project path and local app name;
- supervisor PID and application PID while active;
- state: `starting`, `running`, `stopping`, `stopped`, or `error`;
- start command fingerprint, not the full environment;
- start time, exit time, and last exit code;
- random launch nonce.

The supervisor does not automatically restart an application. When the application exits on its own, the supervisor atomically records the final stopped/error state, removes the control socket, releases the lock, and exits. The final state remains available to later status calls.

## Start Protocol

The public CLI:

1. Resolves and validates every requested target.
2. Derives the state paths and attempts the app lock.
3. If the lock is held, queries the control socket. A healthy running supervisor makes start an idempotent success. An unresponsive owner produces a bounded error, not a duplicate process.
4. If the lock is acquired, writes a `0600` startup payload containing the command, directory, canonical identity, nonce, and timeouts.
5. Opens `output.log` in append mode and creates a readiness pipe.
6. Starts the current Nova executable in a hidden supervisor mode, passing the lock and readiness descriptors through `ExtraFiles`. The command itself is not exposed in process-list arguments.
7. Waits for bounded readiness. Success means the supervisor owns the lock, has created its socket, and has started the application process.
8. Releases the supervisor process handle and returns immediately.

For `all`, apps start in declaration order. If a later new start fails, Nova stops only supervisors newly started by that invocation; supervisors that were already running remain untouched.

## Supervisor Process Model

The supervisor starts in a new session detached from the invoking terminal. It launches `sh -lc <start>` in a separate application process group. Application stdout/stderr append to `output.log`; stdin is `/dev/null`.

The supervisor owns exactly one application process group. It concurrently waits for:

- application exit;
- a stop request on the Unix control socket;
- SIGINT or SIGTERM directed at the supervisor.

On stop, it records `stopping`, sends TERM to the full application process group, waits three seconds, sends KILL to remaining processes, reaps the shell, records the final state, acknowledges the requester, and exits. Every wait has a deadline.

## Stop, Restart, and Run

Stop is idempotent. If the lock is free, Nova removes a stale socket/startup payload, preserves valid final state, and reports already stopped. If the lock is held, Nova validates state identity, connects to the control socket, requests stop, and waits boundedly for acknowledgement and lock release.

Restart and run first preflight every requested start command, stop every selected identity in declaration order, then start every target in declaration order. If cleanup cannot be confirmed, Nova does not start a replacement over uncertain ownership.

## Status

Local status never infers process health from PID existence alone.

- If the lock is held and the socket responds with matching identity, report the live supervisor state.
- If the lock is free, report the last valid final state from `state.json`.
- If no state exists, report `state=not_started`.
- If the lock is held but the socket does not respond before the timeout, return an explicit `state=unknown` error rather than falsely reporting running or stopped.

Output is one stable line per target:

```text
app=api state=running pid=12345 started=2026-08-26T10:00:00+08:00 exited=- exit=-
```

For `all`, lines follow YAML declaration order. Status returns exit code 0 for valid running, stopped, error, and not-started observations; inability to establish trustworthy ownership/status is an operational error.

## Logs

The supervisor captures application stdout/stderr in `output.log`, and `nova start` prints that path. This release does not change `nova logs`; it remains remote-only. A later change may expose local log reading without changing supervisor ownership.

## Error Handling and Security

- Configuration and bootstrap failures return code 1.
- Supervisor startup and communication failures identify project/app and relevant state path without printing environment values or Agent tokens.
- Control requests and responses are bounded JSON messages with schema version and launch nonce.
- State identity and nonce are validated before sending signals.
- Stale PID data without a held matching lock is never signaled.
- Unix is the supported local supervisor platform; release builds remain Linux/macOS amd64/arm64.

## Code Organization

- `internal/project`: start-only lifecycle schema, identity resolution, inheritance, and validation.
- `internal/localsupervisor`: cache paths, atomic state, locks, control protocol, detached launch, process-group cleanup, start/stop/restart/status orchestration.
- `cmd/nova`: local/remote parsing and routing plus the hidden supervisor entry point.
- `internal/localcommand`: removed after supervisor execution fully replaces synchronous lifecycle command execution.

## Testing

Implementation follows red-green-refactor and uses real short-lived shell processes for lifecycle behavior. Coverage includes:

- start-only schema, stop-key migration errors, inheritance, identity-only stop/status resolution, and `all` preflight;
- safe hashed paths, atomic state, exclusive locks, and selector traversal resistance;
- detached start returning promptly while the app remains alive;
- readiness failures, duplicate start, working directory/environment, and log capture;
- stop of running/already-stopped apps and complete descendant cleanup;
- forced kill after grace timeout;
- spontaneous exit and persisted final exit state;
- restart/run stop-all then start-all ordering and partial-start rollback;
- running, stopped, error, not-started, and unresponsive-owner status;
- default local status, explicit `status --remote`, and Agent-bootstrap bypass;
- no remote deploy/status/log regressions;
- vet, full tests, race tests, and four target cross-builds.

## Migration and Release

Migration from `v0.1.14`:

- Keep `start`, but change it to a long-running foreground command if it currently daemonizes.
- Remove local `stop`; Nova now owns termination.
- Continue adding `--remote` for Agent lifecycle/status operations.

After verification, set version metadata to `0.2.0`, update README/spec/changelog, push main, tag `v0.2.0`, wait for the Release workflow, and verify all four binaries plus `SHA256SUMS.txt`.

## Out of Scope

- Global daemon installation.
- Automatic restart or crash loops.
- Local `nova logs` routing.
- Health probes beyond supervisor/process ownership.
- Windows support.
- Multiple processes per app or dependency graphs.
