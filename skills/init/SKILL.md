---
name: init
description: Establish or maintain a repository's Nova Run project and deployment configuration, then optionally bind existing local Agent credentials. Use when onboarding a project or changing its Nova-managed build, artifact, or lifecycle declaration; do not use to deploy, provision infrastructure, or configure cloud services.
metadata:
  novaRunRelease: "0.2.9"
---

# Nova Run Init

Confirm that these instructions match the installed CLI before changing the project. Read `metadata.novaRunRelease` and run `nova --version`. If Nova is missing or its version differs, report the mismatch and stop before writing `nova.yaml` or local credentials.

Resolve the intended repository root, then inspect its existing `nova.yaml`, manifests, build scripts, entrypoints, and output conventions. Preserve existing human-authored configuration and comments where practical. Do not infer commands or application boundaries from an empty directory. Establish from project evidence, or ask for any material decision that remains unclear:

- a stable lowercase project id using letters, numbers, and single hyphens;
- each application or selector and any intentionally different remote app name;
- a repeatable build command and exactly one deployable artifact path per application;
- the long-running local foreground `start` command, when local lifecycle management is wanted; and
- the production `service.command`, relative to the artifact root, when the artifact runs as a service.

Treat Nova Run and every application framework, compiler, Artifact format, and Runtime as independent systems. Nova executes the repository-owned `build.commands`, packages the configured path, and runs the declared command; it does not infer another product's project model, load its Toolchain Adapter, or require product-specific integration merely because the repository contains that product's manifest. A framework manifest proves application intent, not a Nova build command. If the repository has no repeatable command that produces the required deployable path, report the missing project build entry and ask for that project decision; do not diagnose a Nova compatibility defect or invent an import of another product's private modules.

When repository evidence explicitly identifies an EV protocol 3 project, `ev check`, `ev build [app]`, `ev dev <app>` and `ev start <app> [--prod]` remain EV-owned public commands. The check and build commands are valid Nova `build.commands`, and an explicitly selected EV start command may be used as a Nova lifecycle or service command. Nova still treats every one as an opaque shell command: EV reads Application Descriptions, selects Toolchain and Runtime Adapters, builds Artifacts, and defines development/start behavior; Nova neither repeats nor interprets those steps.

Keep secrets, tokens, machine-specific addresses, and cloud credentials out of `nova.yaml`. Use a top-level `project` and either one top-level application or an `apps` mapping. Nova treats a configured artifact directory as the packaged root; for a single-file artifact with a service, it stages that file's basename at the packaged root. For a service directory Nova adds its own `run` and `nova.app.yaml` files at that root. If a framework's Artifact is an immutable or closed directory, the project build must place it below an outer Nova package directory, and `service.command` must invoke it through that nested path. Never point Nova at the closed inner directory and then mistake Nova's outer packaging files for framework integration. Write `service.command` for the final packaged layout, not for the source-repository path. For example:

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

For an opaque immutable Runtime Artifact, the equivalent shape can be `dist/example/artifact/` inside the configured outer path `dist/example`, with a command such as `framework-runtime ./artifact`; the exact builder, directory name, Runtime command, and environment still require repository evidence.

For multiple applications, put the same `start`, `build`, `artifacts`, and `service` fields below `apps.<selector>`. The selector already becomes the remote app name; add `app` only when the remote name must differ. `start` must remain in the foreground and must not daemonize. `service.command` is the production entrypoint and need not equal `start`. Do not add database, cache, domain, proxy, TLS, load-balancer, discovery, or cloud-resource configuration to Nova's model.

Create or update `nova.yaml` narrowly after the required facts are known. Check the resulting YAML and paths against the repository evidence. `nova target list` may verify the project and an existing credential binding. Do not use `nova status`, `nova deploy`, lifecycle mutations, or a build as an initialization check.

Only after `nova.yaml` is valid, optionally bind this project to an already available Nova Agent by running `nova init`. Treat that as a separate local credential write: do it only when the person explicitly asked for binding and the Endpoint and project token are already available through a secure input path. Never place the token in a command line, file in the repository, logs, or the final report; if secure input is unavailable, hand the interactive `nova init` step to the person. Verify a completed binding with `nova target list` without revealing the token.

Stop after project configuration and any explicitly requested local binding. Initialization does not authorize deployment, process changes, Agent installation, authorization rotation, Aliyun changes, or any other infrastructure mutation.
