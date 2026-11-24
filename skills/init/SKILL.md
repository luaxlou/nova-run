---
name: init
description: Establish or maintain a repository's Nova Run project and deployment configuration, then optionally bind existing local Agent credentials. Use when onboarding a project or changing its Nova-managed build, artifact, or lifecycle declaration; do not use to deploy, provision infrastructure, or configure cloud services.
metadata:
  novaRunRelease: "0.2.15"
---

# Nova Run Init

Confirm that these instructions match the installed CLI before changing the project. Read `metadata.novaRunRelease` and run `nova --version`. If Nova is missing or its version differs, report the mismatch and stop before writing `nova.yaml` or local credentials.

Resolve the intended repository root, then inspect its existing `nova.yaml`, manifests, build scripts, entrypoints, and output conventions. Preserve existing human-authored configuration and comments where practical. Do not infer commands or application boundaries from an empty directory. Establish from project evidence, or ask for any material decision that remains unclear:

- a stable lowercase project id using letters, numbers, and single hyphens;
- each application or selector and any intentionally different remote app name;
- a repeatable build command and exactly one deployable artifact path per application;
- the long-running local foreground `start` command, when local lifecycle management is wanted; and
- the production `service.command`, relative to the artifact root, when the artifact runs as a service; and
- the optional `service.platform`, currently `linux-x64`, when a standalone executable must be rejected on an incompatible Agent host.

Treat Nova Run and every application framework, compiler, Artifact format, and Runtime as independent systems. Nova executes repository-owned public commands, packages the configured path, and runs the declared process; it does not infer another product's project model, load its adapters, or claim a product relationship merely because the repository contains another manifest. A product manifest proves application intent, not a Nova build command. If the repository has no repeatable public command that produces the required deployable path, report the missing project fact and ask for that decision; do not diagnose a compatibility defect or invent an import of private modules.

Identify command ownership from the repository's authoritative instructions and the installed command's help. Prefer the highest-level public entry that completely owns the requested phase. When a public build command already selects its compiler, adapter, or packager and produces the declared Artifact, configure that command alone; do not append its internal package-manager, compiler, copy, or wrapper steps. Treat its output directory as opaque. Separate environments by intent: local `start` must use the product's documented development entry, while `service.command` must use its documented production entry from the delivered Artifact root. Do not substitute a production start for local development merely because it happens to run, and do not use a development watcher as the production process.

Nova's local supervisor discovers the ports actually opened by the foreground process. Do not add a fixed port, address, or another product's host-control environment variables merely so Nova can report or supervise that process. When the public development entry already chooses or reports its own binding, configure that entry alone. If a repository wrapper only injects another product's internal host controls before delegating to its public development command, treat the wrapper as leaked implementation detail. If the wrapper also establishes application-owned state, credentials, identity, or other required development facts, do not silently remove it: report that the repository lacks one complete public development entry and ask the project to establish that boundary first.

Keep secrets, tokens, machine-specific addresses, product-private host controls, and cloud credentials out of `nova.yaml`. Use a top-level `project` and either one top-level application or an `apps` mapping. For a single-file service artifact, Nova stages that file's basename at the package root. For a directory service artifact, Nova preserves the configured directory as an opaque `artifact/` subtree inside its own deployment wrapper and executes `service.command` with that subtree as the working directory; the project must not add a second packaging command merely to accommodate Nova metadata. Write `service.command` relative to the configured Artifact root, not the source-repository path. For example:

```yaml
project: example
app: example
start: exec ./bin/example
build:
  commands:
    - go build -o dist/example ./cmd/example
artifacts:
  - dist/example
service:
  command: ./example
```

For multiple applications, put the same `start`, `build`, `artifacts`, and `service` fields below `apps.<selector>`. The selector already becomes the remote app name; add `app` only when the remote name must differ. `start` must remain in the foreground and must not daemonize. `service.command` is the production entrypoint and need not equal `start`. Do not add database, cache, domain, proxy, TLS, load-balancer, discovery, or cloud-resource configuration to Nova's model.

Create or update `nova.yaml` narrowly after the required facts are known. Check the resulting YAML and paths against the repository evidence. `nova target list` may verify the machine-wide target already shared by every project. Do not create a project-specific Endpoint or credential file, and do not use `nova status`, `nova deploy`, lifecycle mutations, or a build as an initialization check.

Only when `nova target list` proves that this machine has no global target, optionally configure the already available Nova Agent by running `nova init`. Treat that as one machine-level credential write, normally to `~/.nova/client.env`: do it only when the person explicitly asked for target configuration and the Endpoint and machine token are already available through a secure input path. Never place the token in a command line, repository file, logs, or the final report; if secure input is unavailable, hand the interactive `nova init` step to the person. Verify the global target with `nova target list` without revealing the token. A target already used by another project must be reused rather than duplicated.

Stop after project configuration and any explicitly requested local binding. Initialization does not authorize deployment, process changes, Agent installation, authorization rotation, Aliyun changes, or any other infrastructure mutation.
