---
title: API Reference
weight: 20
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

# API Reference

flowd exposes a REST API for programmatic access to jobs, runs, artifacts, and system information. All endpoints return JSON unless otherwise specified.

## Base URL

When running in serve mode, the API is available at:

```
http://localhost:8080/api/v1
```

## Authentication

Authentication and authorization are handled via the Security Profiles system. See the [Configuration]({{< ref "configuration" >}}) documentation for details on setting up API access.

## Endpoints

### Jobs

#### List Jobs

```http
GET /api/v1/jobs
```

Returns a list of all discovered jobs across all sources.

**Query Parameters:**
- `source` (optional): Filter by source name
- `namespace` (optional): Filter by namespace

**Response:**
```json
{
  "jobs": [
    {
      "id": "backup/daily",
      "name": "daily",
      "namespace": "backup",
      "version": "1.0.0",
      "description": "Daily backup job",
      "source": "local-fs"
    }
  ]
}
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
  "source": "local-fs"
}
```

### Runs

#### Create Run

```http
POST /api/v1/runs
```

Creates and executes a new run of a job.

**Request Body:**
```json
{
  "job_id": "backup/daily",
  "args": {
    "target": "/mnt/backup"
  },
  "tenant": "default",
  "async": true
}
```

**Response:**
```json
{
  "run_id": "run_01HX...",
  "job_id": "backup/daily",
  "status": "running",
  "created_at": "2024-01-15T10:30:00Z"
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
{
  "runs": [
    {
      "run_id": "run_01HX...",
      "job_id": "backup/daily",
      "status": "success",
      "created_at": "2024-01-15T10:30:00Z",
      "finished_at": "2024-01-15T10:35:00Z"
    }
  ],
  "total": 42
}
```

#### Get Run Details

```http
GET /api/v1/runs/{run_id}
```

Returns detailed information about a specific run.

**Response:**
```json
{
  "run_id": "run_01HX...",
  "job_id": "backup/daily",
  "status": "success",
  "created_at": "2024-01-15T10:30:00Z",
  "finished_at": "2024-01-15T10:35:00Z",
  "args": {
    "target": "/mnt/backup"
  },
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

Returns the health status of the flowd instance.

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
      "type": "fs",
      "path": "/opt/flowd/jobs"
    }
  ],
  "extensions": ["tui", "mcp"]
}
```

## Server-Sent Events (SSE)

flowd supports real-time event streaming via Server-Sent Events for monitoring runs and system events.

### Run Events Stream

```http
GET /api/v1/events/runs
```

Streams all run-related events.

**Query Parameters:**
- `run_id` (optional): Filter events for a specific run
- `job_id` (optional): Filter events for a specific job

**Event Types:**
- `run.created`: New run started
- `run.started`: Run execution began
- `run.output`: Log output from run
- `run.finished`: Run completed
- `step.started`: Step execution began
- `step.output`: Log output from step
- `step.finished`: Step completed

**Example Event:**
```
event: run.output
data: {"run_id":"run_01HX...","timestamp":"2024-01-15T10:30:01Z","level":"info","message":"Processing..."}
```

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
