---
name: doctor
description: Diagnose a Nova Run project's local configuration, CLI compatibility, credential binding, and read-only status, then apply narrowly scoped local repairs only with explicit authorization. Do not use it to deploy, change process state, mutate cloud resources, or perform implicit external writes.
metadata:
  novaRunRelease: "0.2.15"
---

# Nova Run Doctor

Doctor is read-only by default. Invoking this Skill authorizes inspection, not repair.

First read `metadata.novaRunRelease`, locate `nova`, and run `nova --version`. If the CLI is missing or its version differs, explain the mismatch before interpreting project behavior; do not install or upgrade it implicitly.

Resolve the intended repository root and inspect the existing `nova.yaml`. If it is absent, report that the project has not been initialized and recommend Nova Run Init; do not create a configuration merely to make Doctor pass. Read the repository's manifests, build scripts, entrypoints, artifact conventions, and Git status far enough to evaluate the declared configuration without executing project code.

Keep Nova Run separate from application frameworks and Runtime products while diagnosing. Nova only executes configured public commands, packages selected output, and launches the declared process; it does not understand an external manifest, load its adapters, or claim a product relationship. A missing repository build command is a missing project fact, not evidence that native integration is required.

Determine each configured command's role from authoritative repository instructions and the installed command's help, without interpreting another product's private model. A high-level public build entry that already owns compilation, adapter selection, packaging, and Artifact formation must not be followed by its internal package-manager, compiler, copy, or wrapper command. Diagnose that duplication as leaked implementation detail even if it currently succeeds. Treat the declared Artifact path as opaque. Verify that local `start` uses the documented development entry and `service.command` uses the documented production entry from the delivered Artifact root; a working production command in the local field is still an environment error, and a development watcher is not a production service.

Nova discovers the ports actually opened by each supervised local process. Diagnose fixed ports, addresses, or another product's host-control environment variables in `start` when they exist only to help Nova observe the process; they are unnecessary coupling. Prefer the other product's complete public development command when it owns binding selection. Inspect a repository wrapper far enough to distinguish a redundant launcher from one that also creates application-owned development state, credentials, identity, or other required facts. The former is leaked implementation detail; the latter proves that the repository does not yet have one complete public development entry and is not safe to replace automatically.

Check the facts in their natural dependency order:

- the project and app identities are valid and unambiguous;
- every selector resolves to a repeatable build command and exactly one plausible artifact path;
- local `start` commands are long-running foreground public development entries, contain no obsolete `run` or `stop` fields, and do not pin product-private host controls merely for Nova supervision;
- each configured `service.command` is valid relative to the declared Artifact root and contains no repository secret; remember that a directory service artifact is preserved below Nova's outer deployment wrapper and used as the command working directory, while a single-file service artifact is staged at the wrapper root under its basename;
- any immutable or closed directory Artifact is selected directly, so Nova's generic outer deployment wrapper can preserve its contents and use it as the service working directory;
- referenced paths and scripts agree with the repository, distinguishing an expected not-yet-built output from a broken path; and
- the machine has one readable global Nova target without exposing its token; do not diagnose a missing project-specific binding when another project already uses that target.

Use only commands that preserve the diagnosed project and external systems. `nova target list` checks the machine-wide Nova target without revealing its token. The default target lives at `~/.nova/client.env` and is shared by all projects; `NOVA_CLIENT_ENV` may explicitly select another global target file. Do not run `nova status` as a default configuration check because it creates local supervisor cache files. If runtime status is part of the requested scope, explain that local side effect before running it. `nova status --remote` may also trigger credential setup when no target exists, so use it only after `nova target list` succeeds and the person has requested a network connectivity check. Never print credential-file contents or tokens.

Do not run `nova init`, `nova deploy`, installers, build commands, or `nova start`, `stop`, `restart`, or `run` during the read-only phase. Do not edit files, change permissions, rotate credentials, contact cloud control planes, or alter Aliyun, Agent, systemd, firewall, DNS, proxy, database, or cache state.

Explain the result as a coherent account of what is working, what concrete fact is inconsistent, its consequence, and the smallest safe local repair. Do not turn internal checks into a wall of labels. Before any repair, name the exact local file, field, or permission that would change and obtain explicit authorization for that repair. A general request to “run doctor” is not repair authorization.

After authorization, preserve unrelated content and apply only the agreed local repair. Do not guess missing build commands, app boundaries, artifact paths, development commands, production commands, Endpoints, or credentials. Do not recommend modifying Nova merely because another product owns its build or Runtime contract; first identify that product's public command and output boundary without asserting any integration. Ambiguous configuration needs a decision, not an automatic fix. Never broaden a local repair into deployment, process control, credential rotation, Agent configuration, or cloud-resource work.

Rerun the relevant read-only checks after a repair and report what now works and what remains. A healthy Doctor result does not authorize a build, deployment, release, or cleanup.
