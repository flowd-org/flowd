---
title: API Reference
weight: 20
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

# API Reference

flwd exposes a REST API for programmatic access to jobs, runs, artifacts, and system information. All endpoints return JSON unless otherwise specified.

## Base URL

When running in serve mode, the API is available at:

```
http://localhost:8080/api/v1
```

## Authentication

Authentication and authorization are handled via the Security Profiles system. See the [Configuration]({{< ref "configuration" >}}) documentation for details on setting up API access.

The following probe endpoints are intentionally public (no bearer token required):

- `GET /healthz`
- `GET /startupz`
- `GET /readyz`

`GET /metrics` may also be unauthenticated when the server binds to a loopback address (for other bind addresses, it requires a bearer token with the usual scopes).

All other endpoints in this document require authentication with the appropriate scopes.

## Endpoints

### Operational probes and introspection

#### Startup probe

```http
GET /startupz
```

Readiness for process startup completion.

- `204 No Content`: startup sequence complete.
- `503 Service Unavailable`: startup is incomplete.
  - Problem `type`: `https://flowd.org/problems/startup-incomplete`

#### Readiness probe

```http
GET /readyz
```

Readiness for serving traffic.

- `204 No Content`: Core DB and storage checks are healthy.
- `503 Service Unavailable`: server is not ready.
  - Problem `type`: `https://flowd.org/problems/not-ready`
  - Includes a `checks` extension with per-subsystem status.

#### Runtime limits

```http
GET /limits
```

Returns runtime scheduling limits and queue defaults.

- Auth required.
- Required scope: `jobs:read`

Example response:

```json
{
  "algorithm": "wfq",
  "concurrency": 8,
  "queue_max_depth": 1024,
  "backpressure_mode": "reject_when_full",
  "queue_stats": {
    "len": 0,
    "enqueued": 0,
    "dequeued": 0,
    "dropped": 0
  },
  "updated_at": "2026-02-11T10:00:00Z"
}
```

#### Server capabilities

```http
GET /capabilities
```

Returns core identity/version plus compiled and enabled extension metadata.

- Auth required.
- Required scope: `jobs:read`

Example response:

```json
{
  "core": {
    "version": "1.0.0",
    "spec_version": "1.0.1",
    "app_id": "flwd"
  },
  "extensions": [
    {
      "name": "export",
      "version": "1.0.0",
      "compiled": true,
      "enabled": false
    }
  ]
}
```

### Jobs

#### List Jobs

```http
GET /api/v1/jobs
```

Returns a list of all discovered jobs across all sources.

Job IDs are always returned as canonical slash IDs. When a job is defined at the source root and `mountPath` is `.`, the canonical job ID is the empty string (`""`).

**Query Parameters:**
- `source` (optional): Filter by source name
- `namespace` (optional): Filter by namespace

**Response:**
```json
[
  {
    "id": "backup/daily",
    "name": "backup/daily",
    "tenant": "default",
    "origin": {
      "source_kind": "fs",
      "source_name": "local"
    },
    "description": "Daily backup job",
    "source": {
      "name": "local-fs",
      "type": "local"
    }
  }
]
```

#### Get Job Details

```http
GET /api/v1/jobs/{job_id}
```

Returns detailed information about a specific job, including its configuration and argument schema.

**Response:**
```json
{
  "id": "backup/daily",
  "name": "daily",
  "namespace": "backup",
  "version": "1.0.0",
  "description": "Daily backup job",
  "tenant": "default",
  "origin": {
    "source_kind": "fs",
    "source_name": "local-fs"
  },
  "args": {
    "type": "object",
    "properties": {
      "target": {
        "type": "string",
        "description": "Backup target directory"
      }
    },
    "required": ["target"]
  },
  "source": {
    "name": "local-fs"
  }
}
```

### Runs

#### Create Run

```http
POST /api/v1/runs
```

Creates and executes a new run of a job.

Job references are input-compatible (aliases, existing dot-form IDs, and case-insensitive slash IDs), but responses, persistence, SSE, and journal entries always use canonical slash IDs.

##### Idempotency

`POST /api/v1/runs` requires an idempotency key so clients can safely retry without duplicate effects.

**Required header**
- `Idempotency-Key`: 20–128 characters, alphanumeric plus `_` and `-`.

**Optional header**
- `Idempotency-SHA256`: lowercase hex SHA-256 of the canonicalized JSON request body.
  - If provided and it does not match the server-computed hash, the request fails with a conflict.

**TTL behavior**
- Default TTL: **24 hours**.
- Maximum TTL: **72 hours** (higher configured values are clamped to this maximum).

**Replay behavior**
- If the same `Idempotency-Key` and request body are replayed after a successful run creation, the server returns the cached response and sets `Idempotent-Replay: true`.

**Tenant scoping**
- Idempotency keys are scoped by the resolved tenant. The same key in different tenants does not collide.

**Failure modes (RFC7807)**
- **409 Conflict** — key reuse with a different request body or hash mismatch:
  - `type`: `https://flowd.org/problems/idempotency/mismatch`
- **429 Too Many Requests** — key already in use (in-flight):
  - `type`: `https://flowd.org/problems/scheduler/rejected`
- **400 Bad Request** — missing/invalid `Idempotency-Key` or invalid `Idempotency-SHA256` format.

**Request Body:**
```json
{
  "job_id": "backup/daily",
  "args": {
    "target": "/mnt/backup"
  },
  "tenant": "default",
  "source": {"name": "local-fs"}
}
```

**Tenant resolution rules (summary)**
- If a principal tenant claim is present, it is authoritative.
- If both a principal tenant and a request `tenant` are present and differ, the request is rejected (RFC7807).
- If no principal tenant is available, `tenant` defaults to `default` when omitted.
- If the principal exists but has no tenant claim, treat it as "no principal tenant" for resolution.

**Root job aliases**
- If the resolved canonical job ID is `""`, `job_id` may be sent as `""`, `"."`, or `"/"` and will be normalized to `""`.

**Response:**
```json
{
  "id": "run_01HX...",
  "job_id": "backup/daily",
  "tenant": "default",
  "origin": {
    "source_kind": "fs",
    "source_name": "local-fs"
  },
  "status": "running",
  "started_at": "2024-01-15T10:30:00Z"
}
```

#### List Runs

```http
GET /api/v1/runs
```

Returns a list of all runs.

**Query Parameters:**
- `job_id` (optional): Filter by job ID
- `status` (optional): Filter by status (`pending`, `running`, `success`, `failed`)
- `limit` (optional): Maximum number of results (default: 100)
- `offset` (optional): Pagination offset

**Response:**
```json
[
  {
    "id": "run_01HX...",
    "job_id": "backup/daily",
    "tenant": "default",
    "origin": {
      "source_kind": "fs",
      "source_name": "local-fs"
    },
    "status": "success",
    "started_at": "2024-01-15T10:30:00Z",
    "finished_at": "2024-01-15T10:35:00Z"
  }
]
```

#### Get Run Details

```http
GET /api/v1/runs/{run_id}
```

Returns detailed information about a specific run.

**Response:**
```json
{
  "id": "run_01HX...",
  "job_id": "backup/daily",
  "tenant": "default",
  "origin": {
    "source_kind": "fs",
    "source_name": "local-fs"
  },
  "status": "success",
  "started_at": "2024-01-15T10:30:00Z",
  "finished_at": "2024-01-15T10:35:00Z",
  "result": {
    "value": {
      "files_backed_up": 1234,
      "total_size_mb": 5678
    }
  }
}
```

#### Get Run Logs

```http
GET /api/v1/runs/{run_id}/logs
```

Returns the logs for a specific run.

**Query Parameters:**
- `follow` (optional): Stream logs in real-time (SSE)
- `since` (optional): Return logs since timestamp

**Response (JSON):**
```json
{
  "logs": [
    {
      "timestamp": "2024-01-15T10:30:01Z",
      "level": "info",
      "message": "Starting backup...",
      "step_id": "step_01"
    }
  ]
}
```

**Response (SSE when `follow=true`):**
```
event: log
data: {"timestamp":"2024-01-15T10:30:01Z","level":"info","message":"Starting backup..."}

event: log
data: {"timestamp":"2024-01-15T10:30:02Z","level":"info","message":"Backup complete"}
```

#### Cancel Run

```http
POST /api/v1/runs/{run_id}/cancel
```

Cancels a running job.

**Response:**
```json
{
  "run_id": "run_01HX...",
  "status": "cancelled"
}
```

### Artifacts

#### List Artifacts

```http
GET /api/v1/artifacts
```

Returns a list of all artifacts.

**Query Parameters:**
- `run_id` (optional): Filter by run ID
- `limit` (optional): Maximum number of results

**Response:**
```json
{
  "artifacts": [
    {
      "id": "artifact_01HX...",
      "run_id": "run_01HX...",
      "name": "backup-archive",
      "path": "/workspace/backup.tar.gz",
      "media_type": "application/gzip",
      "size_bytes": 12345678,
      "created_at": "2024-01-15T10:35:00Z"
    }
  ]
}
```

#### Get Artifact

```http
GET /api/v1/artifacts/{artifact_id}
```

Downloads the artifact file.

**Response:**
Binary content with appropriate `Content-Type` header.

### System

#### Health Check

```http
GET /api/v1/health
```

Returns the health status of the flwd instance.

**Response:**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime_seconds": 3600
}
```

#### Get System Info

```http
GET /api/v1/system/info
```

Returns system information and configuration.

**Response:**
```json
{
  "version": "1.0.0",
  "sources": [
    {
      "name": "local-fs",
      "type": "local",
      "ref": "/opt/flwd/jobs"
    }
  ],
  "extensions": ["tui", "mcp"]
}
```

## Server-Sent Events (SSE)

flwd supports real-time event streaming via Server-Sent Events for monitoring runs and system events.

### Endpoints

```http
GET /events/stream
```

Global stream of all events visible to the caller.

```http
GET /events
```

Alias of `/events/stream`.

```http
GET /runs/{run_id}/events
```

Run-scoped stream for a single run.

### Envelope and retry behavior

All SSE endpoints use a single `flowd` envelope and a fixed retry hint:

```
event: flowd
retry: 3000
id: 42
data: {"seq":42,"ts":"2026-01-10T10:00:15Z","type":"run.output","run_id":"run_01HX...","tenant":"default","origin":{"source_kind":"fs","source_name":"local-fs"},"data":{...}}
```

Heartbeat comments are emitted at least every 15 seconds in the form:

```
:hb 2026-01-10T10:00:15Z
```

### Resume and stale cursor behavior

Resume by sending the last received sequence number via `Last-Event-ID`:

```http
GET /events/stream
Last-Event-ID: 41
```

- If the cursor is older than the retained journal range, the server responds with **HTTP 410** and RFC7807 type `https://flowd.org/problems/sse/stale-cursor`.
- If the cursor is not a valid integer or is greater than the current max sequence, the server responds with **HTTP 400**.

### Event types (minimum set)

- `run.started`
- `run.output`
- `run.finished`
- `step.started`
- `step.output`
- `step.finished`
- `log`
- `metric`
- `warning`
- `error`
- `source.sync`
- `policy.denied`

## Error Handling

All API errors follow RFC 7807 Problem Details format:

```json
{
  "type": "https://flowd.org/problems/job-not-found",
  "title": "Job Not Found",
  "status": 404,
  "detail": "The job 'backup/daily' does not exist",
  "instance": "/api/v1/jobs/backup/daily"
}
```

Common error types:
- `job-not-found` (404): Requested job does not exist
- `run-not-found` (404): Requested run does not exist
- `validation-error` (400): Invalid request parameters
- `execution-error` (500): Job execution failed
- `permission-denied` (403): Insufficient permissions

## Rate Limiting

API requests may be rate-limited based on the security profile configuration. Rate limit information is included in response headers:

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1642248000
```

## Versioning

The API version is included in the URL path (`/api/v1`). Breaking changes will result in a new API version.
