---
title: "Serve Mode (HTTP + SSE)"
weight: 5
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

Run a local server that mirrors CLI capabilities over HTTP and Server-Sent
Events (SSE). This is useful for CI runners, small servers, gateways and any
setup where other systems need to call jobs remotely.

## Start the server

Start a server bound to localhost:

```bash
$ flwd :serve --bind 127.0.0.1:8080 --profile secure --dev
```

- `--bind` sets the listen address.
- `--profile` chooses the default security profile: `secure` (default),
  `permissive` or `disabled`.
- `--dev` enables a development token and permissive CORS for
  `http://localhost` during local experiments.

In production you should:

- avoid `--dev`,
- configure proper TLS termination or a reverse proxy,
- provision real tokens with appropriate scopes.

## Basic HTTP usage

List jobs:

```bash
$ curl -s -H 'Authorization: Bearer dev-token' \
    http://127.0.0.1:8080/jobs | jq
```

Plan a job:

```bash
$ curl -s -X POST http://127.0.0.1:8080/plans \
    -H 'Authorization: Bearer dev-token' \
    -H 'Content-Type: application/json' \
    -d '{"job":"hello-world","args":{"name":"Alice"}}' | jq
```

Submit a run:

```bash
$ curl -s -X POST http://127.0.0.1:8080/runs \
    -H 'Authorization: Bearer dev-token' \
    -H 'Content-Type: application/json' \
    -d '{"job":"hello-world","args":{"name":"Alice"}}' | jq
```

Stream events for a run:

```bash
$ curl -Ns -H 'Authorization: Bearer dev-token' \
    http://127.0.0.1:8080/runs/RUN_ID/events
```

Events are sent as SSE and can be parsed by dashboards, CLIs or monitoring tools.

## Authentication and scopes

Serve mode uses bearer tokens (JWTs) for authentication and simple scopes for
authorisation. Examples of scopes:

- `runs:read`, `runs:write`
- `jobs:read`
- `sources:read`, `sources:write`
- `metrics:read`
- `export:read`

The development mode uses a fixed token (`dev-token`) with broad scopes. In a
real deployment you generate and sign tokens yourself, then configure flwd to
verify them.

## Working with sources

You can register additional sources (local paths, git repositories, OCI add-ons)
via the HTTP API. For example, adding a git source:

```bash
$ curl -s -X POST http://127.0.0.1:8080/sources \
    -H 'Authorization: Bearer dev-token' \
    -H 'Content-Type: application/json' \
    -d '{"type":"git","name":"tools","url":"file:///home/me/repos/tools","ref":"main"}'
```

Jobs from that source will appear in `/jobs` and can be referenced by name or
with a `source` hint in `/plans` and `/runs`.

For full details see [Sources (Local, Git)]({{< ref "sources.md" >}}) and
[OCI Add‑On Sources]({{< ref "oci-addons.md" >}}).

## Server configuration

Configuration is typically provided via a file (for example
`/etc/flwd/server.yaml`). A simplified example:

```yaml
bind: 0.0.0.0:8080
default_profile: secure
db_path: /var/lib/flwd/flwd.db

auth:
  issuer: "https://auth.example.org/"
  audience: "flwd"
  jwk_file: /etc/flwd/jwks.json

container:
  runtime: auto
  verify_signatures: permissive
  allowed_registries:
    - ghcr.io

limits:
  max_concurrent_runs: 32
  max_run_history: 10000
```

The actual options may evolve; see the reference configuration and release notes
for the version you deploy.

All important decisions (policy evaluation, profile downgrades, failures) are
logged and surfaced as events so you can debug behaviour and feed it into
observability pipelines.
