---
title: Job Configuration (config.yaml)
weight: 22
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

# Job Configuration (config.yaml)

Every flowd job requires a `config.yaml` file that defines its metadata, execution settings, arguments, and behavior.

## File Location

The `config.yaml` file must be placed in the job's directory within a source's `tree-v1` structure:

```
jobs/
└── backup/
    └── daily/
        ├── config.yaml    # Job configuration
        └── run.sh         # Job script
```

## Basic Configuration

Here's a minimal `config.yaml` example:

```yaml
name: "Daily Backup"
description: "Performs daily backup of critical data"
executor: "proc"
ulc_profile: "ulc.shell.bash"
command: "./run.sh"
```

## Configuration Fields

### Metadata

```yaml
# Human-readable name
name: "Daily Backup"

# Detailed description
description: |
  Performs daily backup of critical data to remote storage.
  Includes compression and encryption.
```

### Executor

The `executor` field determines how the job runs:

```yaml
# Process executor (default) - runs directly on host
executor: "proc"
command: "./run.sh"

# OR

# Container executor - runs in OCI container
executor: "container"
image: "ghcr.io/org/backup-tool:v1.0.0@sha256:..."
command: "/app/backup"
```

**Executor Types:**
- `proc` (default): Runs as a process on the host system
- `container`: Runs in an OCI container (requires `image` field)

### ULC Profile

Specifies the Universal Language Contract profile (runtime environment):

```yaml
# Bash (default)
ulc_profile: "ulc.shell.bash"

# OR

# PowerShell
ulc_profile: "ulc.shell.pwsh"

# OR

# Python (if available)
ulc_profile: "ulc.python.3"
```

**Built-in Profiles:**
- `ulc.shell.bash` - Bash ≥ 4.0 (default)
- `ulc.shell.pwsh` - PowerShell ≥ 7.0

### Arguments (ArgSpec)

Define typed arguments for your job using JSON Schema 2020-12:

```yaml
argspec:
  type: "object"
  properties:
    target:
      type: "string"
      description: "Backup target directory"
      default: "/mnt/backup"
    
    compress:
      type: "boolean"
      description: "Enable compression"
      default: true
    
    retention_days:
      type: "integer"
      description: "Number of days to retain backups"
      minimum: 1
      maximum: 365
      default: 7
  
  required:
    - "target"
```

**Supported Types:**
- `string`, `integer`, `number`, `boolean`
- `array` (with `items` schema)
- `object` (with `properties` schema)

**Secret Arguments:**

```yaml
argspec:
  properties:
    api_key:
      type: "string"
      format: "secret"  # Marks as secret (redacted in logs)
      description: "API key for remote storage"
```

### Composition Modes

#### Single (Default)

Runs one execution unit:

```yaml
composition: "single"
command: "./run.sh"
```

#### Steps (DAG)

Runs multiple steps with dependencies:

```yaml
composition: "steps"

steps:
  - id: "prepare"
    name: "Prepare Environment"
    script: "./scripts/prepare.sh"
  
  - id: "backup"
    name: "Run Backup"
    script: "./scripts/backup.sh"
    needs:
      - "prepare"
  
  - id: "verify"
    name: "Verify Backup"
    script: "./scripts/verify.sh"
    needs:
      - "backup"
  
  - id: "cleanup"
    name: "Cleanup"
    script: "./scripts/cleanup.sh"
    needs:
      - "verify"
```

**Step Fields:**
- `id` (required): Unique identifier within the job
- `name`: Human-readable name
- `script` (required): Path to step script (relative to job directory)
- `needs`: Array of step IDs this step depends on

### Security Profile

Override the instance's default security profile:

```yaml
security_profile: "secure"  # or "permissive" or "disabled"
```

See [Configuration]({{< ref "configuration#security-profiles-detail" >}}) for profile details.

### Execution Profile

Control execution privileges:

```yaml
# Standard (default) - rootless and sandboxed
exec_profile: "standard"

# OR

# Privileged - requires policy approval
exec_profile: "privileged"
```

### Timeouts

Set execution time limits:

```yaml
timeouts:
  # Overall run timeout
  run: "1h"
  
  # Per-step timeout (for composition: "steps")
  step: "15m"
```

### Artifacts

Configure artifact handling:

```yaml
artifacts:
  # Immutable (default) - unique keys per run
  mode: "immutable"
  default_key_pattern: "backups/{run_id}"

# OR

  # Mutable - reusable keys as pointers
  mode: "mutable"
  default_key_pattern: "state/latest"
```

**Modes:**
- `immutable`: Keys should be unique per run (e.g., `backups/run_123`)
- `mutable`: Keys can be reused as mutable pointers (e.g., `state/latest`)

### Service Bindings

Declare dependencies on Session Services:

```yaml
serviceBindings:
  - "postgres"
  - "redis"
```

The job will have access to these services via environment variables and network connectivity.

## Complete Example: Process Executor

```yaml
name: "Database Backup"
description: "Backs up PostgreSQL database to S3"

executor: "proc"
ulc_profile: "ulc.shell.bash"
command: "./backup.sh"

argspec:
  type: "object"
  properties:
    database:
      type: "string"
      description: "Database name"
      default: "production"
    
    s3_bucket:
      type: "string"
      description: "S3 bucket for backups"
    
    encryption_key:
      type: "string"
      format: "secret"
      description: "Encryption key"
  
  required:
    - "s3_bucket"
    - "encryption_key"

security_profile: "secure"

timeouts:
  run: "30m"

artifacts:
  mode: "immutable"
  default_key_pattern: "db-backups/{run_id}"

serviceBindings:
  - "postgres"
```

## Complete Example: Container Executor

```yaml
name: "Build Container Image"
description: "Builds and pushes Docker image"

executor: "container"
image: "docker:24-dind@sha256:..."
command: "/usr/local/bin/build.sh"

argspec:
  type: "object"
  properties:
    dockerfile:
      type: "string"
      description: "Path to Dockerfile"
      default: "./Dockerfile"
    
    tag:
      type: "string"
      description: "Image tag"
    
    registry_token:
      type: "string"
      format: "secret"
      description: "Registry authentication token"
  
  required:
    - "tag"
    - "registry_token"

exec_profile: "privileged"  # Required for Docker-in-Docker

security_profile: "permissive"

timeouts:
  run: "1h"
```

## Complete Example: Multi-Step Job

```yaml
name: "Deploy Application"
description: "Builds, tests, and deploys application"

executor: "proc"
ulc_profile: "ulc.shell.bash"
composition: "steps"

steps:
  - id: "build"
    name: "Build Application"
    script: "./scripts/build.sh"
  
  - id: "test-unit"
    name: "Run Unit Tests"
    script: "./scripts/test-unit.sh"
    needs:
      - "build"
  
  - id: "test-integration"
    name: "Run Integration Tests"
    script: "./scripts/test-integration.sh"
    needs:
      - "build"
  
  - id: "deploy-staging"
    name: "Deploy to Staging"
    script: "./scripts/deploy.sh"
    needs:
      - "test-unit"
      - "test-integration"
  
  - id: "smoke-test"
    name: "Run Smoke Tests"
    script: "./scripts/smoke-test.sh"
    needs:
      - "deploy-staging"

argspec:
  type: "object"
  properties:
    version:
      type: "string"
      description: "Application version to deploy"
    
    environment:
      type: "string"
      enum: ["staging", "production"]
      default: "staging"
  
  required:
    - "version"

serviceBindings:
  - "docker-registry"

timeouts:
  run: "2h"
  step: "30m"

security_profile: "permissive"
```

## Argument Binding

Arguments defined in `argspec` are automatically bound to shell variables in your job scripts:

### Bash Binding

```yaml
argspec:
  properties:
    target:
      type: "string"
    count:
      type: "integer"
    enabled:
      type: "boolean"
    tags:
      type: "array"
      items:
        type: "string"
```

In your Bash script:

```bash
#!/usr/bin/env bash

# String argument
echo "Target: $target"

# Integer argument (declared with -i)
echo "Count: $count"

# Boolean argument (literal true/false)
if [ "$enabled" = "true" ]; then
  echo "Enabled"
fi

# Array argument (Bash array)
for tag in "${tags[@]}"; do
  echo "Tag: $tag"
done
```

### PowerShell Binding

```yaml
argspec:
  properties:
    target:
      type: "string"
    count:
      type: "integer"
```

In your PowerShell script:

```powershell
# String argument
Write-Host "Target: $Target"

# Integer argument (typed as [int])
Write-Host "Count: $Count"
```

## Validation

Validate your job configuration:

```bash
flowd jobs validate /path/to/job
```

## Next Steps

- [Sources & Tree-v1 Structure]({{< ref "sources-structure" >}}) - Understand job discovery
- [Configuration]({{< ref "configuration" >}}) - Instance-level settings
- [API Reference]({{< ref "api-reference" >}}) - Programmatic job execution
