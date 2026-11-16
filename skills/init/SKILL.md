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

Keep secrets, tokens, machine-specific addresses, and cloud credentials out of `nova.yaml`. Use a top-level `project` and either one top-level application or an `apps` mapping. Nova treats a configured artifact directory as the packaged root; for a single-file artifact with a service, it stages that file's basename at the packaged root. Write `service.command` for that packaged layout, not for the source-repository path. For example:

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

Create or update `nova.yaml` narrowly after the required facts are known. Check the resulting YAML and paths against the repository evidence. `nova target list` may verify the project and an existing credential binding. Do not use `nova status`, `nova deploy`, lifecycle mutations, or a build as an initialization check.

Only after `nova.yaml` is valid, optionally bind this project to an already available Nova Agent by running `nova init`. Treat that as a separate local credential write: do it only when the person explicitly asked for binding and the Endpoint and project token are already available through a secure input path. Never place the token in a command line, file in the repository, logs, or the final report; if secure input is unavailable, hand the interactive `nova init` step to the person. Verify a completed binding with `nova target list` without revealing the token.

Stop after project configuration and any explicitly requested local binding. Initialization does not authorize deployment, process changes, Agent installation, authorization rotation, Aliyun changes, or any other infrastructure mutation.
