---
title: Add-on Manifests
weight: 23
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

# Add-on Manifests

Add-ons are packaged sets of jobs distributed as OCI container images. Each add-on includes a `manifest.yaml` file that defines the jobs, their configurations, and runtime requirements.

## What are Add-ons?

Add-ons allow you to:
- **Package jobs** with their dependencies in OCI images
- **Pin versions** using digest-based image references
- **Distribute tools** via container registries
- **Ensure reproducibility** across environments

## Manifest Location

The manifest must be located at `/addon/manifest.yaml` inside the OCI image:

```
/addon/
├── manifest.yaml    # Add-on manifest (required)
└── scripts/         # Job scripts and resources
    ├── backup.sh
    └── restore.sh
```

## Basic Manifest

Here's a minimal `manifest.yaml`:

```yaml
name: "backup-tools"
version: "1.0.0"
description: "Database backup and restore utilities"

entrypoints:
  - id: "backup"
    name: "Database Backup"
    description: "Backs up database to S3"
    command: ["/addon/scripts/backup.sh"]
    
  - id: "restore"
    name: "Database Restore"
    description: "Restores database from S3"
    command: ["/addon/scripts/restore.sh"]
```

## Manifest Schema

### Metadata

```yaml
# Add-on name (used in job IDs)
name: "backup-tools"

# Semantic version
version: "1.0.0"

# Human-readable description
description: "Database backup and restore utilities"

# Optional metadata
metadata:
  author: "ops-team"
  license: "MIT"
  homepage: "https://github.com/org/backup-tools"
```

### Entrypoints

Each entrypoint defines a job exposed by the add-on:

```yaml
entrypoints:
  - id: "backup"
    name: "Database Backup"
    description: "Backs up PostgreSQL database"
    
    # Command to execute (required)
    command: ["/addon/scripts/backup.sh"]
    
    # Optional: Arguments schema
    argspec:
      type: "object"
      properties:
        database:
          type: "string"
          description: "Database name"
        s3_bucket:
          type: "string"
          description: "S3 bucket for backups"
      required:
        - "database"
        - "s3_bucket"
    
    # Optional: Resource hints
    resources:
      cpu: "1.0"
      memory: "2GiB"
    
    # Optional: Environment variables
    env:
      BACKUP_TOOL_VERSION: "2.0"
    
    # Optional: Network access
    network: "all"  # or "none"
```

### Entrypoint Fields

#### Required Fields

- **`id`** (string): Unique identifier within the add-on
- **`command`** (array): Command to execute in the container

#### Optional Fields

- **`name`** (string): Human-readable name
- **`description`** (string): Detailed description
- **`argspec`** (object): JSON Schema for job arguments
- **`resources`** (object): Resource hints (CPU, memory)
- **`env`** (object): Environment variables
- **`network`** (string): Network access (`all` or `none`)
- **`annotations`** (object): Custom metadata

## Job ID Resolution

Jobs from add-ons are exposed with IDs based on the source's `mountPath`:

```yaml
# In flwd.yaml
sources:
  - name: "backup-addon"
    type: "oci"
    image: "ghcr.io/org/backup-tools:v1.0.0@sha256:..."
    mountPath: "addons/backup"
```

Resulting job IDs:
- `addons/backup/backup` (from entrypoint `id: "backup"`)
- `addons/backup/restore` (from entrypoint `id: "restore"`)

## Resource Hints

Specify resource requirements for jobs:

```yaml
entrypoints:
  - id: "heavy-job"
    command: ["/app/process"]
    
    resources:
      # CPU limit (cores)
      cpu: "2.0"
      
      # Memory limit
      memory: "4GiB"
      
      # Execution timeout
      timeout: "1h"
```

{{< callout type="warning" >}}
Resource hints are **advisory**. The actual limits are determined by the instance's policy engine and may be stricter than requested.
{{< /callout >}}

## Environment Variables

Define environment variables for your jobs:

```yaml
entrypoints:
  - id: "app"
    command: ["/app/run"]
    
    env:
      APP_ENV: "production"
      LOG_LEVEL: "info"
      FEATURE_FLAG_X: "enabled"
```

## Network Access

Control network access for jobs:

```yaml
entrypoints:
  - id: "isolated-job"
    command: ["/app/process"]
    network: "none"  # No network access
  
  - id: "api-job"
    command: ["/app/api"]
    network: "all"   # Full network access
```

## Annotations

Add custom metadata using annotations:

```yaml
entrypoints:
  - id: "db-backup"
    command: ["/app/backup"]
    
    annotations:
      # DB Shim usage hints
      flowd.db.required: "true"
      flowd.db.quota: "1GiB"
      
      # Custom metadata
      team: "platform"
      sla: "critical"
```

## Complete Example

```yaml
name: "postgres-tools"
version: "2.1.0"
description: "PostgreSQL backup, restore, and maintenance tools"

metadata:
  author: "database-team"
  license: "Apache-2.0"
  homepage: "https://github.com/org/postgres-tools"

entrypoints:
  - id: "backup"
    name: "PostgreSQL Backup"
    description: "Creates compressed backup of PostgreSQL database"
    command: ["/app/backup.sh"]
    
    argspec:
      type: "object"
      properties:
        database:
          type: "string"
          description: "Database name to backup"
        
        s3_bucket:
          type: "string"
          description: "S3 bucket for backup storage"
        
        compression:
          type: "string"
          enum: ["gzip", "zstd", "none"]
          default: "zstd"
        
        encryption_key:
          type: "string"
          format: "secret"
          description: "Encryption key for backup"
      
      required:
        - "database"
        - "s3_bucket"
        - "encryption_key"
    
    resources:
      cpu: "2.0"
      memory: "4GiB"
      timeout: "2h"
    
    env:
      PGBACKUP_VERSION: "2.1.0"
      COMPRESSION_LEVEL: "3"
    
    network: "all"
    
    annotations:
      flowd.db.required: "true"
      team: "database"
      sla: "high"
  
  - id: "restore"
    name: "PostgreSQL Restore"
    description: "Restores database from backup"
    command: ["/app/restore.sh"]
    
    argspec:
      type: "object"
      properties:
        database:
          type: "string"
          description: "Target database name"
        
        backup_key:
          type: "string"
          description: "S3 key of backup to restore"
        
        encryption_key:
          type: "string"
          format: "secret"
          description: "Decryption key"
      
      required:
        - "database"
        - "backup_key"
        - "encryption_key"
    
    resources:
      cpu: "2.0"
      memory: "4GiB"
      timeout: "3h"
    
    env:
      PGRESTORE_VERSION: "2.1.0"
    
    network: "all"
  
  - id: "vacuum"
    name: "Database Maintenance"
    description: "Runs VACUUM ANALYZE on database"
    command: ["/app/vacuum.sh"]
    
    argspec:
      type: "object"
      properties:
        database:
          type: "string"
          description: "Database to maintain"
        
        full:
          type: "boolean"
          description: "Run VACUUM FULL"
          default: false
      
      required:
        - "database"
    
    resources:
      cpu: "1.0"
      memory: "1GiB"
      timeout: "1h"
    
    network: "none"
```

## Building Add-on Images

### Dockerfile Example

```dockerfile
FROM alpine:3.18

# Install dependencies
RUN apk add --no-cache bash postgresql-client aws-cli

# Copy manifest
COPY manifest.yaml /addon/manifest.yaml

# Copy scripts
COPY scripts/ /addon/scripts/
RUN chmod +x /addon/scripts/*.sh

# Set working directory
WORKDIR /addon
```

### Build and Push

```bash
# Build image
docker build -t ghcr.io/org/postgres-tools:v2.1.0 .

# Get digest
docker push ghcr.io/org/postgres-tools:v2.1.0
# Output: sha256:abc123...

# Tag with digest
docker tag ghcr.io/org/postgres-tools:v2.1.0 \
  ghcr.io/org/postgres-tools:v2.1.0@sha256:abc123...
```

## Configuring Add-on Sources

Add the add-on to your `flwd.yaml`:

```yaml
sources:
  - name: "postgres-tools"
    type: "oci"
    # MUST use digest-pinned reference
    image: "ghcr.io/org/postgres-tools:v2.1.0@sha256:abc123..."
    mountPath: "tools/postgres"
```

## Security Considerations

### Executor Enforcement

{{< callout type="warning" >}}
Add-on jobs **always** use `executor: "container"`. This cannot be overridden by overlays or tenants.
{{< /callout >}}

### Security Profile

Add-on jobs default to the `secure` security profile unless explicitly configured otherwise in the manifest.

### Image Verification

- Images **must** be pinned by digest (`@sha256:...`)
- Signature verification follows the instance's container trust policy
- Verification failures cause the source to fail-closed

### Resource Limits

Resource hints from the manifest are passed through the policy engine. The effective limits may be stricter than requested but will never exceed policy ceilings.

## Job Origin Metadata

Jobs from add-ons include extended origin information:

```json
{
  "origin": {
    "source_kind": "oci",
    "source_name": "postgres-tools",
    "addon_name": "postgres-tools",
    "addon_version": "2.1.0",
    "addon_image_digest": "sha256:abc123..."
  }
}
```

This metadata is available in:
- Job listings (`GET /jobs`)
- Run records (`GET /runs/{id}`)
- Journal entries

## Validation

Validate your manifest before building:

```bash
# Validate manifest structure
flowd addons validate /path/to/manifest.yaml

# Test add-on locally
flowd addons test /path/to/image
```

## Next Steps

- [Sources & Tree-v1 Structure]({{< ref "sources-structure" >}}) - Understand source configuration
- [Job Configuration]({{< ref "job-configuration" >}}) - Learn about job config.yaml
- [Configuration]({{< ref "configuration" >}}) - Configure OCI sources
