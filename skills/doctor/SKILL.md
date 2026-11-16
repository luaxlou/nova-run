---
name: doctor
description: Diagnose a Nova Run project's local configuration, CLI compatibility, credential binding, and read-only status, then apply narrowly scoped local repairs only with explicit authorization. Do not use it to deploy, change process state, mutate cloud resources, or perform implicit external writes.
metadata:
  novaRunRelease: "0.2.9"
---

# Nova Run Doctor

Doctor is read-only by default. Invoking this Skill authorizes inspection, not repair.

First read `metadata.novaRunRelease`, locate `nova`, and run `nova --version`. If the CLI is missing or its version differs, explain the mismatch before interpreting project behavior; do not install or upgrade it implicitly.

Resolve the intended repository root and inspect the existing `nova.yaml`. If it is absent, report that the project has not been initialized and recommend Nova Run Init; do not create a configuration merely to make Doctor pass. Read the repository's manifests, build scripts, entrypoints, artifact conventions, and Git status far enough to evaluate the declared configuration without executing project code.

Keep Nova Run separate from application frameworks and Runtime products while diagnosing. Nova only executes the configured repository command, packages its selected output, and launches the declared process; it does not need to understand an external manifest or implement that product's Toolchain Adapter. A missing repository build command is a missing project fact, not evidence that Nova and the external product require native integration.

For an explicitly identified EV protocol 3 project, accept `ev check`, `ev build [app]`, `ev dev <app>` and `ev start <app> [--prod]` as EV-owned public commands. Diagnose their availability, lifecycle role and output paths like any other configured command; do not reinterpret their manifests or move Toolchain Adapter, development, or start responsibility into Nova.

Check the facts in their natural dependency order:

- the project and app identities are valid and unambiguous;
- every selector resolves to a repeatable build command and exactly one plausible artifact path;
- local `start` commands are long-running foreground commands and contain no obsolete `run` or `stop` fields;
- each configured `service.command` is valid relative to the packaged artifact and contains no repository secret; remember that a directory's contents become the packaged root, a single-file service artifact is staged there under its basename, and Nova adds `run` plus `nova.app.yaml` to a service directory root;
- any immutable or closed third-party Artifact remains nested below an outer Nova package root, so Nova's packaging files do not alter that inner Artifact and the service command addresses the nested path;
- referenced paths and scripts agree with the repository, distinguishing an expected not-yet-built output from a broken path; and
- the current project has a readable local target binding without exposing its token.

Use only commands that preserve the diagnosed project and external systems. `nova target list` checks the local project binding without revealing its token. Do not run `nova status` as a default configuration check because it creates local supervisor cache files. If runtime status is part of the requested scope, explain that local side effect before running it. `nova status --remote` may also trigger credential setup when no binding exists, so use it only after `nova target list` succeeds and the person has requested a network connectivity check. Never print credential-file contents or tokens.

Do not run `nova init`, `nova deploy`, `nova auth set`, installers, build commands, or `nova start`, `stop`, `restart`, or `run` during the read-only phase. Do not edit files, change permissions, rotate credentials, contact cloud control planes, or alter Aliyun, Agent, systemd, firewall, DNS, proxy, database, or cache state.

Explain the result as a coherent account of what is working, what concrete fact is inconsistent, its consequence, and the smallest safe local repair. Do not turn internal checks into a wall of labels. Before any repair, name the exact local file, field, or permission that would change and obtain explicit authorization for that repair. A general request to “run doctor” is not repair authorization.

After authorization, preserve unrelated content and apply only the agreed local repair. Do not guess missing build commands, app boundaries, artifact paths, production commands, Endpoints, or credentials. Do not recommend modifying Nova merely because an external product has its own build or Runtime contract; first identify the repository-owned command and outer package layout that connect the two independent systems. Ambiguous configuration needs a decision, not an automatic fix. Never broaden a local repair into deployment, process control, credential rotation, Agent configuration, or cloud-resource work.

Rerun the relevant read-only checks after a repair and report what now works and what remains. A healthy Doctor result does not authorize a build, deployment, release, or cleanup.
