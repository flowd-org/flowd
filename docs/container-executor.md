---
title: "Container Executor"
weight: 8
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

Run jobs inside rootless containers with secure defaults where possible.

The container executor lets you package the logic of a job inside a container
image, while flwd handles planning, arguments, profiles and policies.

## Configure a container-backed job

A minimal example job spec might look like:

```yaml
id: demo-container
executor: container
interpreter: "container:alpine:3.20"

args:
  - name: name
    type: string
    required: true
```

At runtime, flwd will:

- ensure a container runtime is available (Podman preferred, Docker supported),
- pull and cache the image if needed,
- start a rootless container with a hardened profile (see below),
- pass arguments and environment according to the Universal Language Contract.

If no supported runtime is found, runs fail fast with
`container.runtime.unavailable`.

## Secure defaults

When running with the `secure` profile on a well-characterised Linux runtime,
the container executor aims for:

- rootless containers,
- read-only root filesystem,
- `--cap-drop=ALL`,
- `--security-opt=no-new-privileges`,
- `--network none` (or equivalent),
- isolated working directories.

Secrets are materialised as files under a dedicated directory such as
`/run/secrets`, and are not leaked via environment variables unless policy
explicitly allows it.

Policies can be configured to allow writable rootfs, extra capabilities or
limited networking in permissive or disabled profiles. All such decisions are
logged and surfaced as events.

## Running and cancelling container jobs

From the CLI, container-backed jobs look like any other job:

```bash
$ flwd demo-container --name "Alice"
```

From the API:

```bash
$ curl -s -X POST http://127.0.0.1:8080/runs \
    -H 'Authorization: Bearer dev-token' \
    -H 'Content-Type: application/json' \
    -d '{
          "job":"demo-container",
          "args":{"name":"Alice"}
        }' | jq
```

To cancel an active run, call:

```bash
$ curl -s -X POST http://127.0.0.1:8080/runs/RUN_ID:cancel \
    -H 'Authorization: Bearer dev-token'
```

The engine will attempt to stop the container gracefully and then forcefully if
needed.

## Troubleshooting

Common issues:

- `container.runtime.unavailable`: install Podman or Docker and ensure it is
  reachable for the user running `flwd`.
- `container.name.conflict`: an orphaned container is blocking reuse; remove it
  manually (for example `podman rm -f NAME`).
- `E_OCI`: the runtime failed to pull or start the image; check its logs and
  your network configuration.

See also [OCI Add‑On Sources]({{< ref "oci-addons.md" >}}) for packaging jobs and dependencies
as images.
