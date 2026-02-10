---
title: "Sources (Local, Git)"
weight: 6
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

Add additional job trees and run tools from them just like local ones.

## Concepts

flwd can load jobs from:

- the local filesystem (relative or absolute paths),
- git repositories (checked out into a local cache),
- OCI add-ons (container images containing job trees).

This page focuses on local and git sources. OCI add-ons are covered separately
in [OCI Add‑On Sources]({{< ref "oci-addons.md" >}}).

When a source is configured, its jobs appear in `/jobs` and are available to the
CLI and TUI. API calls to `/plans` and `/runs` can optionally include a `source`
hint to resolve ambiguous names.

`GET /sources` returns a Core-aligned view of sources, including `tenant`, `kind`, `mountPath`, `layout`, `pull_policy`, and `config`. The `kind` value `oci` is a flowd extension to the Core source kind set. Secret-shaped values inside `config` are redacted (rendered as `REDACTED`).

This response is additive for compatibility: legacy fields such as `name`, `type`, `ref`, `url`, `resolved_ref`, `resolved_commit`, `expose`, and `aliases` (when enabled) may also appear.

`mountPath` in this view reflects the local materialized path for the source (for example, a checked-out git directory or cached OCI manifest directory).

## Add a local source (API)

Assuming `flwd :serve` is running:

```bash
$ curl -s -X POST http://127.0.0.1:8080/sources \
    -H 'Authorization: Bearer dev-token' \
    -H 'Content-Type: application/json' \
    -d '{
          "type":"local",
          "name":"local-tools",
          "ref":"/home/me/tools"
        }'
```

After this, any jobs discovered under `/home/me/tools` are visible via `/jobs`
and can be run like any other job.

```bash
$ curl -s -H 'Authorization: Bearer dev-token' http://127.0.0.1:8080/sources | jq
```

Example response (redacted):

```json
[
  {
    "id": "local-tools",
    "name": "local-tools",
    "type": "local",
    "ref": "/home/me/tools",
    "tenant": "default",
    "kind": "fs",
    "mountPath": "/home/me/tools",
    "layout": "tree-v1",
    "pull_policy": "always",
    "config": {
      "path": "/home/me/tools"
    }
  },
  {
    "id": "backup-addon",
    "name": "backup-addon",
    "type": "oci",
    "ref": "ghcr.io/org/backup-tools:v1.0.0",
    "tenant": "default",
    "kind": "oci",
    "mountPath": "/var/lib/flwd/sources/oci/backup-addon",
    "layout": "",
    "pull_policy": "ifNotPresent",
    "digest": "sha256:...",
    "config": {
      "ref": "ghcr.io/org/backup-tools:v1.0.0",
      "digest": "sha256:...",
      "pull_policy": "ifNotPresent"
    }
  }
]
```

## Add a Git source (API)

Add a git repository as a source:

```bash
$ curl -s -X POST http://127.0.0.1:8080/sources \
    -H 'Authorization: Bearer dev-token' \
    -H 'Content-Type: application/json' \
    -d '{
          "type":"git",
          "name":"tools",
          "url":"file:///home/me/repos/tools",
          "ref":"main"
        }'
```

flwd will clone or fetch the repository into a local cache and discover jobs
inside the configured job tree. After that:

```bash
$ curl -s -H 'Authorization: Bearer dev-token' \
    http://127.0.0.1:8080/jobs | jq
```

You should see entries with a `source` block indicating they come from the `git`
source you added.

## Source hints on plans and runs

If two sources expose jobs with the same ID, you can disambiguate by passing a
`source` hint when planning or running:

```bash
$ curl -s -X POST http://127.0.0.1:8080/plans \
    -H 'Authorization: Bearer dev-token' \
    -H 'Content-Type: application/json' \
    -d '{
          "job_id":"hello-world",
          "source":{"name":"tools"},
          "args":{"name":"Alice"}
        }'
```

The same pattern applies to `/runs`.

## Updating and removing sources

To update a source, send another `POST /sources` with the same `name` and new
parameters. To remove one, call:

```bash
$ curl -s -X DELETE http://127.0.0.1:8080/sources/tools \
    -H 'Authorization: Bearer dev-token'
```

Removing a source does not delete the underlying files; it simply stops exposing
those jobs via the engine.

For packaging jobs inside container images, see [OCI Add‑On Sources]({{< ref "oci-addons.md" >}}).
