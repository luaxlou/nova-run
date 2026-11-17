# Nova Run Project-Scoped Authorization Design

> Superseded: this design was implemented in Nova 0.2.7 and later reversed because the Agent Endpoint and access token are machine-level Nova target configuration, not project configuration. The current contract is defined by `docs/nova-run-spec.md`: all projects reuse `~/.nova/client.env` and the Agent's `/etc/nova/token` through `/v1/apps/...`.

**Date:** 2026-09-25

## Goal

Replace Nova Agent's machine-wide bearer token with project-scoped authorization. A credential issued to one project must be unable to discover, deploy, delete, start, stop, restart, inspect, or read logs for any other project on the same Agent.

For authorization purposes, one project is one `nova.yaml`. A project may contain one app or multiple apps; all apps declared by that project share the project's authorization boundary.

This is a security-sensitive, breaking protocol change and is targeted at `v0.3.0`.

## Current Problem

The current Agent accepts one token from `/etc/nova/token` for every protected `/v1/apps` route. Possession of that token grants access to every remotely managed app. The CLI also writes Endpoint and Token into one user-level runtime configuration, despite `nova init` being described as project initialization.

Consequences:

- compromising one project's token compromises all projects on the Agent;
- a project can list and operate another project's apps by guessing its app name;
- app names share one global namespace;
- switching projects silently reuses the same credential and endpoint;
- replacing one project's token would currently require replacing the machine-wide token for every project.

Moving the existing global token into each repository would not fix the server-side authorization problem. The Agent must bind authenticated identity to a project and enforce that identity on every resource lookup.

## Selected Model

Each remotely managed project has:

- a project name used directly as its authorization identity;
- exactly one current project token, replaced as a whole when credentials are reset;
- its own logical app namespace and Agent storage namespace;
- project-local CLI target configuration stored outside the repository.

Example:

```yaml
project: billing

apps:
  api:
    app: api
    build:
      commands:
        - go build -o dist/api ./cmd/api
    artifacts:
      - dist/api
    service:
      command: ./api
  worker:
    app: worker
    build:
      commands:
        - go build -o dist/worker ./cmd/worker
    artifacts:
      - dist/worker
    service:
      command: ./worker
```

`project` is the project's name, not a separate generated ID. It is required only for remote operations. Local lifecycle commands continue to work without Agent configuration or project credentials.

Project names must be unique on one Agent. Project names and remote app names are normalized, validated slugs. They may not contain path separators, control characters, `.`/`..` segments, or the reserved runtime-name delimiter. A project name cannot be changed implicitly after remote state exists; renaming is an explicit migration operation.

## Authorization Invariant

For each request, the Agent loads the authorization record for the project name in the path and verifies the bearer token against that record. Successful verification produces a principal for that project. Handlers do not accept a project name without a matching verified principal.

For every project API request:

```text
authenticated_project == path_project == storage_project == runtime_project
```

A missing, malformed, or invalid credential returns `401 unauthorized`. Using project A's token on project B therefore fails against B's verifier without revealing whether either project exists.

The machine administrator credential is not a project credential and is never accepted by normal deploy, lifecycle, status, log, list, or delete handlers. This prevents routine clients from becoming accidental superusers.

## API Contract

Project-scoped operations use a new versioned API rather than changing `/v1/apps` in place:

```text
GET    /v2/projects/{project}/apps
PUT    /v2/projects/{project}/apps/{app}
DELETE /v2/projects/{project}/apps/{app}
POST   /v2/projects/{project}/apps/{app}/start
POST   /v2/projects/{project}/apps/{app}/stop
POST   /v2/projects/{project}/apps/{app}/restart
GET    /v2/projects/{project}/apps/{app}/status
GET    /v2/projects/{project}/apps/{app}/logs
```

Every route except `/health` requires a project token. Listing returns only apps in the authenticated project. Project identity is included in structured logs, but bearer tokens and token hashes are never logged.

The client constructs all remote paths from the resolved project name and app target. It does not expose a flag that can override the project name independently of `nova.yaml`.

## Remote Resource Isolation

Agent storage and runtime identities include both project and app. The current systemd-compatible physical layout encodes the validated pair with a reserved delimiter:

```text
<app-root>/<project>--<app>/
```

The systemd unit identity is derived deterministically from the same validated `(project, app)` pair. It must be collision-free for all accepted names and must never be constructed by concatenating unchecked request text.

This allows two projects to both own an app called `api` without collision. Artifact replacement, deployment metadata, status, logs, delete, and lifecycle control all resolve through one shared project/app identity type; handlers must not assemble filesystem paths or unit names independently.

## Token Format and Storage

Project tokens are opaque random credentials, for example:

```text
nvp_<secret>
```

The secret has at least 256 bits of cryptographic randomness. The Agent stores only a versioned password hash or keyed digest of the token, never the recoverable token. Verification uses constant-time comparison.

Authorization state lives in a root-owned directory, defaulting to `/etc/nova/auth`, with directories mode `0700` and files mode `0600`. Each record contains:

- schema version;
- project name;
- secret verifier;
- update time.

Each project has exactly one authorization record. Reissuing authorization atomically overwrites that record and immediately invalidates the previous token. The Agent does not cache authorization indefinitely.

## Administrative Workflow

Project authorization is one server-local administrative operation:

```text
sudo nova auth set <project>
```

If the project has no authorization record, `auth set` creates it. If one already exists, the command replaces it. The command prints the new full token exactly once; the old token stops working immediately. There is no separate create, issue, list, revoke, label, expiry, or multi-token workflow.

The first implementation deliberately does not expose authorization replacement over the public HTTP API. Server-local administration avoids turning the machine bootstrap credential into a network superuser protocol.

## CLI Credential Resolution

`nova init` becomes genuinely project-specific:

1. Load the nearest `nova.yaml` and resolve its project name.
2. Ask for or read the Agent Endpoint and project token.
3. Validate the pair with a project-scoped Agent request.
4. Store it outside the repository at the user configuration directory under the project name, with mode `0600`.

Conceptual layout:

```text
<user-config>/nova/projects/<project>.yaml
```

The file contains Endpoint and project token. It is not written into `nova.yaml`, `.env`, shell startup files, or the Git worktree. Checkouts with the same project name intentionally share the credential for that OS user.

Resolution order for remote commands is:

1. explicit `NOVA_ENDPOINT` plus `NOVA_TOKEN` environment override, requiring both values;
2. the credential file for the current `nova.yaml` project name;
3. an actionable error instructing the user to run `nova init` in this project.

The CLI must not fall back to another project's credential or the legacy global config. `nova target list` displays the current project and endpoint without printing the secret.

## Failure and Information-Disclosure Behavior

- `401` means no usable authentication was supplied; response text stays generic.
- Unknown app names within the caller's own project return `404`.
- Apps and projects outside the caller's scope are never distinguishable through list, status, logs, lifecycle, deployment, deletion, timing shortcuts, or error details.
- Request logs redact `Authorization` and never include project-token material.
- Authentication failures are rate-limited at the server boundary in a follow-up hardening step; the initial implementation keeps the verifier deliberately bounded and non-amplifying.

## Migration

`v0.3.0` introduces `/v2` and project credentials without treating the old machine token as a project token.

Migration sequence:

1. Add `project` to each repository's `nova.yaml`.
2. Upgrade the Agent and client.
3. Run `sudo nova auth set <project>` on the server.
4. Run `nova init` inside that repository.
5. Redeploy the project's apps into the project-scoped runtime namespace.
6. Verify deploy, logs, and remote lifecycle through `/v2`.

The Agent exposes only `/v2` project authorization. There is no legacy global-token fallback. The installer no longer generates or presents a machine-wide client token.

## Code Organization

- `internal/authz`: project authorization records, atomic replacement, verification, and principals.
- `internal/agent`: `/v2` routing, authentication middleware, scope enforcement, and project-aware handlers.
- `internal/deploy`: a shared validated project/app identity used for storage and metadata paths.
- `internal/runtime`: project-aware systemd unit identity and lifecycle/log commands.
- `internal/project`: `project` schema, validation, and remote identity resolution.
- `internal/client`: `/v2` project-scoped requests and credential-safe errors.
- `cmd/nova`: `nova auth` server-local commands plus project-specific `nova init` and target resolution.

## Verification Requirements

Tests must prove the negative authorization boundary, not only successful access:

- a token for project A can perform every supported action on A;
- the same token cannot list, deploy, delete, start, stop, restart, inspect, or read logs for project B;
- identical app names in two projects resolve to different storage paths and systemd identities;
- malformed and incorrect tokens are rejected;
- verification is constant-time for equal-length secrets and never stores plaintext secrets;
- project/app traversal and ambiguous identifiers are rejected before path construction;
- CLI project A never loads project B's credential or the legacy global token;
- local lifecycle still bypasses all Agent credential loading;
- overwriting project A's authorization invalidates A's old token without changing project B;
- legacy `/v1` is unavailable;
- full tests, race tests, vet, Linux Agent builds, and macOS/Linux CLI builds pass.

An integration test starts one Agent with projects A and B, deploys an `api` app to both, and exercises the complete cross-project denial matrix.

## Out of Scope

- Per-user roles within a project.
- Per-action permissions such as deploy-only or logs-only tokens.
- Organization/team hierarchy, SSO, OAuth, or OIDC.
- Public HTTP APIs for creating projects or issuing credentials.
- Multiple simultaneous tokens, token labels, expiry, token listing, and individual revocation.
- Sharing one app between projects.
- Automatic project renaming or implicit adoption of legacy apps.
- Encrypting artifacts at rest.
