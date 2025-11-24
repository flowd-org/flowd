---
title: Sources & Tree-v1 Structure
weight: 24
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

# Sources & Tree-v1 Structure

Sources define where flowd discovers jobs. Each source uses the **tree-v1** directory layout to organize jobs in a hierarchical structure.

## What are Sources?

Sources are providers of jobs that flowd can discover and execute. They can be:
- **Local filesystem** directories
- **Git repositories** (local or remote)
- **OCI images** (add-ons)

## Source Types

### Filesystem Source

Points to a local directory containing jobs:

```yaml
sources:
  - name: "local-ops"
    type: "fs"
    path: "/opt/flowd/jobs"
    mountPath: "ops"
    watch: true  # Auto-reload on changes
```

**Fields:**
- `name`: Unique identifier for the source
- `type`: Must be `"fs"`
- `path`: Absolute path to the jobs directory
- `mountPath`: Prefix for job IDs (e.g., `ops/backup`)
- `watch`: Enable filesystem watching for auto-reload

### Git Source

Clones and syncs jobs from a Git repository:

```yaml
sources:
  - name: "shared-tools"
    type: "git"
    url: "https://github.com/org/flowd-jobs.git"
    ref: "main"
    mountPath: "shared"
    pull_policy: "on-run"
    auto_sync: true
    poll_interval: "5m"
```

**Fields:**
- `name`: Unique identifier for the source
- `type`: Must be `"git"`
- `url`: Git repository URL
- `ref`: Branch, tag, or commit SHA
- `mountPath`: Prefix for job IDs
- `pull_policy`: When to pull updates (`on-run`, `manual`, `disabled`)
- `auto_sync`: Periodically sync to latest commit (for branches)
- `poll_interval`: How often to check for updates

### OCI Source (Add-ons)

Loads jobs from OCI container images:

```yaml
sources:
  - name: "backup-addon"
    type: "oci"
    image: "ghcr.io/org/backup-tools:v1.0.0@sha256:..."
    mountPath: "addons/backup"
```

**Fields:**
- `name`: Unique identifier for the source
- `type`: Must be `"oci"`
- `image`: OCI image reference (must be digest-pinned)
- `mountPath`: Prefix for job IDs

See [Add-on Manifests]({{< ref "addon-manifests" >}}) for details on OCI add-ons.

## Tree-v1 Directory Layout

The **tree-v1** layout organizes jobs in a hierarchical directory structure where each directory containing a `config.yaml` file defines a job.

### Basic Structure

```
jobs/                    # Source root
├── backup/              # Namespace
│   ├── daily/           # Job directory
│   │   ├── config.yaml  # Job configuration (required)
│   │   └── run.sh       # Job script
│   └── weekly/
│       ├── config.yaml
│       └── run.sh
└── deploy/
    ├── staging/
    │   ├── config.yaml
    │   └── deploy.sh
    └── production/
        ├── config.yaml
        └── deploy.sh
```

### Job ID Resolution

Job IDs are derived from the directory path relative to the source root:

```
Source mountPath: "ops"
Directory path: backup/daily

Resulting job ID: ops/backup/daily
```

**Examples:**

| Source mountPath | Directory Path      | Job ID                    |
|------------------|---------------------|---------------------------|
| `ops`            | `backup/daily`      | `ops/backup/daily`        |
| `tools`          | `deploy/staging`    | `tools/deploy/staging`    |
| `shared`         | `db/migrate`        | `shared/db/migrate`       |
| `.`              | `hello`             | `hello`                   |

### Root Job

A job at the source root (`.`) is allowed:

```
jobs/
└── config.yaml    # Root job
```

This creates a job with ID equal to the `mountPath` (or empty string if `mountPath` is `.`).

## Discovery Process

flowd discovers jobs using the following process:

1. **Mount sources**: Each source is mounted at its `mountPath` under the tenant's scripts root
2. **Walk directories**: Recursively walk the directory tree
3. **Identify jobs**: Any directory containing `config.yaml` is a job
4. **Resolve IDs**: Job ID = `mountPath` + relative directory path
5. **Check collisions**: Fail if multiple jobs resolve to the same ID

### Discovery Example

**Configuration:**

```yaml
sources:
  - name: "local-ops"
    type: "fs"
    path: "/opt/flowd/jobs"
    mountPath: "ops"
  
  - name: "shared-tools"
    type: "git"
    url: "https://github.com/org/tools.git"
    ref: "main"
    mountPath: "shared"
```

**Directory Structure:**

```
/opt/flowd/jobs/          # local-ops source
├── backup/
│   └── daily/
│       └── config.yaml

/tmp/flowd/git/tools/     # shared-tools source (cloned)
├── deploy/
│   └── app/
│       └── config.yaml
```

**Discovered Jobs:**

- `ops/backup/daily` (from local-ops)
- `shared/deploy/app` (from shared-tools)

## Job Collision Detection

If two or more jobs resolve to the same job ID, discovery fails with an error:

```
Error: Job ID collision detected
Job ID: ops/backup/daily
Sources:
  - local-ops (mountPath: ops, path: backup/daily)
  - remote-ops (mountPath: ops, path: backup/daily)
```

**Resolution:**
- Change `mountPath` for one of the sources
- Reorganize directory structure
- Remove duplicate job

## Multi-Step Jobs

Jobs can include multiple script files for step-based execution:

```
jobs/
└── deploy/
    ├── config.yaml       # composition: "steps"
    └── scripts/
        ├── build.sh      # Step 1
        ├── test.sh       # Step 2
        └── deploy.sh     # Step 3
```

**config.yaml:**

```yaml
name: "Deploy Application"
composition: "steps"

steps:
  - id: "build"
    script: "./scripts/build.sh"
  
  - id: "test"
    script: "./scripts/test.sh"
    needs: ["build"]
  
  - id: "deploy"
    script: "./scripts/deploy.sh"
    needs: ["test"]
```

## Organizing Jobs

### By Function

```
jobs/
├── backup/
│   ├── database/
│   ├── files/
│   └── logs/
├── deploy/
│   ├── frontend/
│   └── backend/
└── maintenance/
    ├── cleanup/
    └── optimize/
```

### By Environment

```
jobs/
├── staging/
│   ├── deploy/
│   └── rollback/
└── production/
    ├── deploy/
    └── rollback/
```

### By Team

```
jobs/
├── platform/
│   ├── infrastructure/
│   └── monitoring/
├── data/
│   ├── etl/
│   └── analytics/
└── security/
    ├── audit/
    └── compliance/
```

## Source Management

### Adding Sources

Add sources to `flwd.yaml`:

```yaml
sources:
  - name: "my-jobs"
    type: "fs"
    path: "/path/to/jobs"
    mountPath: "custom"
```

Reload configuration:

```bash
flowd sources reload
```

### Listing Sources

```bash
# List all sources
flowd sources list

# Show source details
flowd sources get my-jobs
```

### Pulling Updates (Git Sources)

```bash
# Manual pull
flowd sources pull shared-tools

# Pull all sources
flowd sources pull --all
```

### Watching for Changes (Filesystem)

Enable `watch: true` for automatic reloading:

```yaml
sources:
  - name: "local-dev"
    type: "fs"
    path: "./jobs"
    mountPath: "dev"
    watch: true  # Auto-reload on file changes
```

## Pull Policies

Control when Git sources are updated:

```yaml
sources:
  - name: "shared-tools"
    type: "git"
    url: "https://github.com/org/tools.git"
    ref: "main"
    pull_policy: "on-run"  # or "manual" or "disabled"
```

**Policies:**
- `on-run`: Pull before each job execution
- `manual`: Only pull when explicitly requested
- `disabled`: Never pull (use initial clone only)

## Auto-Sync (Git Sources)

Periodically sync to the latest commit:

```yaml
sources:
  - name: "shared-tools"
    type: "git"
    url: "https://github.com/org/tools.git"
    ref: "main"
    auto_sync: true
    poll_interval: "5m"  # Check every 5 minutes
```

{{< callout type="warning" >}}
Auto-sync only works with branch references (not tags or commit SHAs).
{{< /callout >}}

## Complete Example

**flwd.yaml:**

```yaml
instance:
  name: "production"
  data_dir: "/var/lib/flowd"

sources:
  # Local development jobs
  - name: "local-dev"
    type: "fs"
    path: "/opt/flowd/dev-jobs"
    mountPath: "dev"
    watch: true
  
  # Shared team jobs from Git
  - name: "platform-tools"
    type: "git"
    url: "https://github.com/org/platform-tools.git"
    ref: "main"
    mountPath: "platform"
    pull_policy: "on-run"
    auto_sync: true
    poll_interval: "10m"
  
  # Production-ready jobs from Git (pinned)
  - name: "prod-ops"
    type: "git"
    url: "https://github.com/org/prod-ops.git"
    ref: "v2.1.0"  # Pinned tag
    mountPath: "ops"
    pull_policy: "manual"
  
  # Backup tools add-on
  - name: "backup-addon"
    type: "oci"
    image: "ghcr.io/org/backup-tools:v1.0.0@sha256:abc123..."
    mountPath: "addons/backup"
```

**Directory Structure:**

```
/opt/flowd/dev-jobs/      # local-dev
├── test/
│   └── hello/
│       ├── config.yaml
│       └── run.sh

/var/lib/flowd/git/platform-tools/  # platform-tools (cloned)
├── deploy/
│   └── app/
│       ├── config.yaml
│       └── deploy.sh

/var/lib/flowd/git/prod-ops/  # prod-ops (cloned)
├── backup/
│   └── database/
│       ├── config.yaml
│       └── backup.sh
```

**Discovered Jobs:**

- `dev/test/hello`
- `platform/deploy/app`
- `ops/backup/database`
- `addons/backup/backup` (from OCI add-on)
- `addons/backup/restore` (from OCI add-on)

## Validation

Validate source configuration:

```bash
# Validate sources in config
flowd config validate

# Test source discovery
flowd sources discover my-jobs
```

## Next Steps

- [Job Configuration]({{< ref "job-configuration" >}}) - Learn about config.yaml
- [Add-on Manifests]({{< ref "addon-manifests" >}}) - Package jobs as OCI images
- [Configuration]({{< ref "configuration" >}}) - Configure sources in flwd.yaml
