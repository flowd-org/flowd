# flowd (Serve Mode + CLI)

This repository hosts the flwd daemon and CLI.

## Licenses

- Core source code: **AGPLv3 or later** (see `LICENSE`).
- Documentation in `docs/`: **CC BY 4.0** (see `docs/LICENSE`).

## Quick Start

```bash
# list local jobs discovered under scripts/**/config.d/config.yaml
flwd :jobs

# preview a plan (human output)
flwd :plan demo

# run a job (events streamed to console)
flwd demo --name alice --tags alpha --secret_token xyz

# stream events (NDJSON via SSE)
flwd demo --name alice --secret_token xyz --json  # streams SSE
```

Run events are journaled in the Core DB. You can stream them live via SSE (`GET /runs/{id}/events`).
Optional, on-demand NDJSON export may be provided for debugging, but on-disk artifact retention is not mandated.
Secrets are redacted from plan previews and events by default.

## Shell Completions

Generate updated completion scripts any time flags or verbs change:

```bash
# Bash (writes to ~/.local/share to source from .bashrc)
flwd completion bash > ~/.local/share/flwd.bash

# Zsh
flwd completion zsh > ~/.local/share/flwd.zsh

# Fish
flwd completion fish > ~/.config/fish/completions/flwd.fish

# PowerShell
flwd completion powershell > $env:USERPROFILE\flwd.ps1
```

Source the generated file (or add it to your shell config) to enable tab-completion.

## Aliases & Completion (Phase 7)

Declare friendly command names in `scripts/flwd.yaml`:

```yaml
aliases:
  - from: demo
    to: hello-demo
    description: Shortcut for demo job
```

Discover and use aliases exactly like native commands:

```bash
flwd :jobs --json | jq '{aliases:.aliases}'
flwd hello-demo --name Avery
```

Diagnostics surface alongside alias metadata—e.g., reserved names appear in `alias_invalid` with canonical RFC7807 codes (`alias.reserved`, `alias.name.conflict`, etc.).

The hidden completion entrypoint emits NDJSON candidates that shell integrations consume. Smoke-test it directly:

```bash
# Segment suggestions (aliases included)
flwd __complete 1 | jq

# Flags & value hints after resolving a command
flwd __complete 4 hello-demo "" | jq
flwd __complete 5 hello-demo -- --loud "" | jq
```

Success criteria SC703 is enforced via `go test ./internal/e2e -run TestCompletionLatencyThresholds`, which generates ≥1 000 completion calls and asserts p50 ≤ 25 ms, p95 ≤ 60 ms, p99 ≤ 120 ms.

Serve mode controls alias visibility with `FLWD_ALIASES_PUBLIC=true` (environment) or `flwd :serve --aliases-public`. Elevated callers holding `sources:write` or `jobs:write` still receive alias metadata when the toggle is disabled.

## Serve Mode

```bash
flwd :serve --bind 127.0.0.1:8080 --profile secure --dev
```

Serve mode exposes REST endpoints and SSE streams that mirror the CLI functionality:

- `GET /jobs`
- `POST /plans`
- `POST /runs`, `GET /runs`, `GET /runs/{id}`
- `GET /runs/{id}/events` (SSE)
- `GET /sources`, `POST /sources`, `GET /sources/{name}`
- `GET /metrics` (Prometheus text exposition)

Auth is required (Bearer token with scopes); `--dev` mode falls back to a default token and enables permissive CORS for `http://localhost`.
`--log=json` emits structured request logs via slog.
`/runs` responses and SSE streams surface live executor events; events are stored in the Core DB journal.

### Metrics

- `GET /metrics` exposes Prometheus text-format metrics covering HTTP RED counters/histograms, `flwd_build_info`, `flwd_security_profile{profile}`, container run/pull histograms, and `flwd_policy_denials_total{reason}`.
- Metrics are unauthenticated only when the server binds to a loopback address (e.g., `127.0.0.1`). For other bind addresses, supply a bearer token with the usual scopes.
- Disable collection via `--metrics=false` if you do not want the endpoint exposed.
- Pair the endpoint with a Prometheus scrape to monitor request rates, policy denials, and container activity.

`flwd demo --name alice` is now effectively equivalent to invoking `POST /runs` followed by streaming via SSE.

### Security Profiles & Policy Bundle

- Profiles: `secure` (default), `permissive`, `disabled`. Choose via `--profile` flag or `FLWD_PROFILE`.
- Precedence: request `requested_security_profile` > `FLWD_PROFILE` > CLI flag/config > default `secure`.
- Provide a policy bundle via `FLWD_POLICY_FILE=/path/to/flwd.policy.yaml`. When the active profile is `secure`, the server runs `cosign verify --keyless $FLWD_POLICY_FILE` during startup and refuses to serve if verification fails.
- Bundle keys (subset):
  ```yaml
  verify_signatures: required | permissive | disabled
  allowed_registries:
    - ghcr.io
  ceilings:
    cpu: 1000m
    memory: 1Gi
  overrides:
    network: ["bridge"]
    rootfs_writable: true
    caps: ["NET_ADMIN"]
    env_inheritance: false
  ```
- Overrides are denied in `secure`, allowed only when listed in `overrides` for `permissive`, and always allowed (but audited) in `disabled`. Every decision is logged and published as an SSE `policy.decision` event containing `run_id`, `subject`, `decision`, `code`, and `reason`.

### Sources

- `GET /jobs` now returns a `source` block for entries coming from configured sources. Jobs from the default scripts root omit this field.
- `POST /runs` and `POST /plans` accept an optional `source` object (`{"name":"<source-name>"}`) to resolve jobs against that source's root.
- Local sources can be registered via the API; paths must fall within the configured allow-list (defaults to the serve mode scripts root).

Example: add an additional job root and run a job from it.

```bash
curl -s -X POST http://127.0.0.1:8080/sources \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dev-token' \
  -d '{"type":"local","name":"extras","ref":"../extra-scripts"}'

curl -s -X POST http://127.0.0.1:8080/runs \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dev-token' \
  -d '{"job_id":"hello","args":{"name":"Alice"},"source":{"name":"extras"}}'
```

Plan and run provenance now echo the configured source metadata under `provenance.source`, including pinned refs and checkout paths.

Registering a git source requires both a repository URL and a ref (branch, tag, or commit). flowd materializes a working tree under the data directory (e.g., `${DATA_DIR}/flowd/sources/<name>/`) and pins the ref to a commit hash:

```bash
curl -s -X POST http://127.0.0.1:8080/sources \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer dev-token' \
  -d '{"type":"git","name":"tools","url":"file:///home/me/repos/tools","ref":"main"}'
```

After the checkout completes, any jobs discovered under `scripts/**/config.d/config.yaml` inside the repository are available (namespaced by source) via `GET /jobs`, `POST /plans`, and `POST /runs`.

For a quick sanity check, run `scripts/smoke-sources.sh` (requires `jq`) against a local `flwd :serve` instance; it registers a source alias, plans a job with the `source` hint, triggers a run, and prints the resulting provenance plus the first few NDJSON events. Set `SOURCE_TYPE=git` alongside `SOURCE_URL` and `GIT_REF` to exercise git-backed jobs:

```bash
SOURCE_TYPE=git \
SOURCE_URL="file:///path/to/repo" \
GIT_REF=main \
JOB_ID=gitjob \
scripts/smoke-sources.sh
```

### OCI Add-on Sources

Runner discovers add-on jobs published as OCI images when the image contains `/flwd-addon/manifest.yaml`.

#### API workflow

```bash
# Add an OCI source (must opt in with trusted=true)
curl -s -X POST http://127.0.0.1:8080/sources \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{
        "type":"oci",
        "name":"addon",
        "ref":"ghcr.io/example/addon:1.0.0",
        "trusted":true,
        "pull_policy":"on-add"
      }'

# Plan a namespaced job exposed by the add-on
curl -s -X POST http://127.0.0.1:8080/plans \
  -H 'Authorization: Bearer dev-token' \
  -H 'Content-Type: application/json' \
  -d '{"job_id":"addon/build"}' | jq .provenance.source

# Remove the add-on
curl -s -X DELETE http://127.0.0.1:8080/sources/addon \
  -H 'Authorization: Bearer dev-token'
```

#### CLI workflow

```bash
# Add the source via CLI
flwd :sources add \
  --type oci \
  --name addon \
  --ref ghcr.io/example/addon:1.0.0 \
  --pull-policy on-add \
  --trusted

# Discover jobs as usual
flwd :plan addon/build --json

# Remove when done
flwd :sources remove addon
```

Pull policy semantics:

- `on-add` (default) pulls the image immediately, pins the resolved digest, and reuses cached layers.
- `on-run` defers pulls until plan/run time. Manifest extraction still runs with `--pull=never`; provenance omits the digest until a run finishes successfully.

#### Manifest extraction flags

| Profile      | Network          | Root filesystem | Additional flags                                        |
|--------------|------------------|-----------------|--------------------------------------------------------|
| `secure`     | `--network none` | `--read-only`   | `--cap-drop=ALL --security-opt=no-new-privileges`      |
| `permissive` | `--network none` | `--read-only`   | Same as `secure`; signature failures produce warnings  |
| `disabled`   | `--network bridge` | runtime default | `--cap-drop=ALL --security-opt=no-new-privileges`; writable rootfs/network allowed |

Policy overrides (e.g., `rootfs_writable`, `network`) granted for permissive/disabled profiles appear in plan/run provenance alongside `policy.decision` events.

#### Troubleshooting add-ons

- `source.trust.required` — resubmit with `"trusted":true` (API) or `--trusted` (CLI).
- `image.registry.not.allowed` — add the registry to the policy bundle `allowed_registries` and restart the server.
- `image.signature.required` — sign the image with `cosign`, switch to a permissive profile for local testing, or relax `verify_signatures` in policy.
- `E_ADDON_MANIFEST` — the manifest is missing or violates the schema; inspect the problem details for the failing field and rebuild the image.
- `E_OCI` — the container runtime failed while pulling or extracting; verify the runtime has network access and the ref exists.

## Container Executor

- Configure a job with `executor: container` and an interpreter like `container:alpine:3.20`.
- `flwd :serve` performs a startup preflight and refuses to boot when neither Podman nor Docker is available; CLI runs fail fast with the same RFC7807 Problem.
- Runner detects Podman (preferred) or Docker on the host before accepting the run; missing runtimes return `422 container.runtime.unavailable`.
- Container runs mount the job directory read-only and the per-run artifact directory read/write, preserving secure defaults (`--cap-drop=ALL`, `--security-opt=no-new-privileges`, `--read-only`, `--network none`).
- Secret arguments are materialized as files under `/run/secrets/<name>` inside the container. Environment variables derived from secrets are deliberately omitted unless policy overrides enable env inheritance.
- When policy allows writable rootfs, capability additions, or network changes in `permissive`/`disabled`, those choices are reflected in the container args and accompanied by `policy.decision` events.
- Active runs can be canceled with `POST /runs/{id}:cancel`. The server maps the request to `podman stop --time=<N>` / `kill`, emits a `run.canceled` SSE event, and responds `202 Accepted` without starting execution for disallowed jobs.
- Example job: see `scripts/demo-container/`.

## Troubleshooting

- **`container.runtime.unavailable`** — Podman/Docker is missing. Install a supported runtime or point the server at a system where one is present before retrying.
- **`container.name.conflict`** — an orphaned container prevents re-use of the deterministic run name. Remove it manually (e.g. `podman rm -f <name>`) or fix the runtime so `rm --force` succeeds.
- **`image.signature.required`** — the target image failed `cosign verify`. Re-sign the image (e.g. `cosign sign --keyless <image>`), adjust the policy bundle’s `verify_signatures` mode, or request a permissive profile if policy allows it.
- **`image.registry.not.allowed` / `source.not.allowed`** — update `allowed_registries` or the git host/path allow-list in the policy bundle (or serve configuration), then restart the server so the new policy is loaded.
- **`source.trust.required`** — OCI sources must opt in; add `trusted=true` (API) or `--trusted` (CLI).
- **`E_ADDON_MANIFEST`** — manifest missing/invalid; see problem detail for fields to fix.
- **`E_OCI`** — runtime pull/extract failure; inspect podman/docker logs and verify registry access.


