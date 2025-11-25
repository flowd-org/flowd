---
title: Configuration (flwd.yaml)
weight: 21
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

# Configuration (flwd.yaml)

flwd uses a YAML configuration file (`flwd.yaml`) to define instance-level settings, sources, security profiles, and remote connections.

## Configuration File Location

By default, flwd looks for configuration in the following locations (in order):

1. `./flwd.yaml` (current directory)
2. `~/.config/flwd/flwd.yaml` (user config)
3. `/etc/flwd/flwd.yaml` (system config)

You can override this with the `--config` flag:

```bash
flwd serve --config /path/to/custom-config.yaml
```

## Basic Configuration

Here's a minimal `flwd.yaml` example:

```yaml
# Instance metadata
instance:
  name: "production"
  data_dir: "/var/lib/flwd"

# Job sources
sources:
  - name: "local-ops"
    type: "fs"
    path: "/opt/flwd/jobs"

# API server settings
server:
  bind: "0.0.0.0:8080"
  read_timeout: "30s"
  write_timeout: "30s"
```

## Configuration Sections

### Instance Settings

```yaml
instance:
  # Instance name (used in logs and metrics)
  name: "production"
  
  # Data directory for persistence (SQLite DB, artifacts)
  data_dir: "/var/lib/flwd"
  
  # Default tenant (if not specified in requests)
  default_tenant: "default"
```

### Sources

Sources define where flwd discovers jobs. See [Sources & Tree-v1 Structure]({{< ref "sources-structure" >}}) for details.

```yaml
sources:
  # Local filesystem source
  - name: "local-ops"
    type: "fs"
    path: "/opt/flwd/jobs"
    watch: true  # Auto-reload on changes
  
  # Git repository source
  - name: "shared-tools"
    type: "git"
    url: "https://github.com/org/flwd-jobs.git"
    ref: "main"
    poll_interval: "5m"
  
  # OCI add-on source
  - name: "backup-addon"
    type: "oci"
    image: "ghcr.io/org/flwd-backup:v1.0.0@sha256:..."
```

### Server Settings

```yaml
server:
  # Bind address and port
  bind: "0.0.0.0:8080"
  
  # Timeouts
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"
  
  # TLS configuration (optional)
  tls:
    enabled: true
    cert_file: "/etc/flwd/tls/cert.pem"
    key_file: "/etc/flwd/tls/key.pem"
```

### Security Profiles

Security profiles control execution permissions and sandboxing. See [Security Profiles](#security-profiles-detail) below for details.

```yaml
security:
  # Default profile for all jobs
  default_profile: "secure"
  
  # Profile definitions
  profiles:
    secure:
      network: "none"
      filesystem: "workspace-only"
      env_passthrough: []
    
    permissive:
      network: "all"
      filesystem: "host"
      env_passthrough: ["PATH", "HOME"]
```

### Logging

```yaml
logging:
  # Log level: debug, info, warn, error
  level: "info"
  
  # Log format: json, text
  format: "json"
  
  # Output destination
  output: "stdout"  # or file path
  
  # Structured fields to include
  fields:
    environment: "production"
    region: "us-east-1"
```

### Persistence

```yaml
persistence:
  # SQLite database path (relative to data_dir)
  db_path: "flwd.db"
  
  # Artifact storage settings
  artifacts:
    # Per-artifact size limit
    max_size: "256MiB"
    
    # Per-tenant quota
    quota_soft: "5GiB"
    quota_hard: "6GiB"
    
    # Default TTL for artifacts
    ttl: "7d"
```

### Scheduler

```yaml
scheduler:
  # Global concurrency limit (default: 2 * GOMAXPROCS, min 4)
  global_concurrency: 8
  
  # Per-job concurrency limit
  job_concurrency: 2
  
  # Queue settings
  queue:
    # Weighted Fair Queuing (WFQ) weights
    default_weight: 1
```

### Extensions

Enable or disable optional extensions:

```yaml
extensions:
  enabled:
    - "tui"        # Terminal UI
    - "mcp"        # Model Context Protocol
    - "triggers"   # Scheduled/webhook triggers
    - "export"     # NDJSON event export
    - "maint"      # Database maintenance
```

### Remotes

Configure connections to remote flwd instances:

```yaml
remotes:
  production:
    base_url: "https://flwd.example.com"
    
    # Authentication
    auth:
      type: "private_key_jwt"
      token_path: "/etc/flwd/auth/token.json"
      private_key_path: "/etc/flwd/auth/key.pem"
      audience: "https://flwd.example.com"
      
      # Additional JWT claims
      claims:
        cid: "ci-runner"
    
    # Caching
    cache:
      etag_ttl: "30s"
```

## Security Profiles Detail

Security profiles define the execution environment and permissions for jobs.

### Profile Fields

```yaml
security:
  profiles:
    profile_name:
      # Network access
      network: "none" | "all" | "restricted"
      
      # Filesystem access
      filesystem: "workspace-only" | "host" | "restricted"
      
      # Environment variable passthrough
      env_passthrough:
        - "PATH"
        - "HOME"
      
      # Allowed capabilities (Linux)
      capabilities:
        - "CAP_NET_BIND_SERVICE"
      
      # Resource limits
      resources:
        cpu_limit: "2.0"
        memory_limit: "2GiB"
        timeout: "1h"
```

### Built-in Profiles

#### `secure` (Default)
- **Network**: None
- **Filesystem**: Workspace only
- **Env**: Clean environment (no passthrough)
- **Use case**: Production jobs, untrusted code

#### `permissive`
- **Network**: All
- **Filesystem**: Host access
- **Env**: Selected variables passed through
- **Use case**: Development, trusted tools

#### `disabled`
- **Network**: All
- **Filesystem**: Full host access
- **Env**: Full environment passthrough
- **Use case**: Legacy scripts, debugging

### Job-Level Override

Jobs can request a specific profile in their `config.yaml`:

```yaml
# In job's config.yaml
security_profile: "permissive"
```

The effective profile is determined by:
1. Job's `security_profile` field
2. Instance's `default_profile`
3. Fallback to `secure`

## Environment Variables

Configuration values can be overridden with environment variables:

```bash
# Instance name
FLOWD_INSTANCE_NAME=staging

# Data directory
FLOWD_DATA_DIR=/var/lib/flowd

# Server bind address
FLOWD_SERVER_BIND=0.0.0.0:9090

# Log level
FLOWD_LOG_LEVEL=debug
```

Environment variables use the format: `FLOWD_<SECTION>_<KEY>` (uppercase, underscores).

## Configuration Validation

Validate your configuration file:

```bash
flwd config validate
```

View the effective configuration (after env var overrides):

```bash
flwd config show
```

## Example: Complete Configuration

```yaml
instance:
  name: "production"
  data_dir: "/var/lib/flwd"
  default_tenant: "default"

sources:
  - name: "ops-tools"
    type: "fs"
    path: "/opt/flwd/jobs"
    watch: true
  
  - name: "shared-library"
    type: "git"
    url: "https://github.com/org/flwd-jobs.git"
    ref: "main"
    poll_interval: "5m"

server:
  bind: "0.0.0.0:8080"
  read_timeout: "30s"
  write_timeout: "30s"
  tls:
    enabled: true
    cert_file: "/etc/flwd/tls/cert.pem"
    key_file: "/etc/flwd/tls/key.pem"

security:
  default_profile: "secure"
  profiles:
    secure:
      network: "none"
      filesystem: "workspace-only"
      env_passthrough: []
    permissive:
      network: "all"
      filesystem: "host"
      env_passthrough: ["PATH", "HOME", "USER"]

logging:
  level: "info"
  format: "json"
  output: "stdout"
  fields:
    environment: "production"

persistence:
  db_path: "flwd.db"
  artifacts:
    max_size: "256MiB"
    quota_soft: "5GiB"
    quota_hard: "6GiB"
    ttl: "7d"

scheduler:
  global_concurrency: 8
  job_concurrency: 2

extensions:
  enabled:
    - "tui"
    - "export"
    - "maint"

remotes:
  staging:
    base_url: "https://staging.flwd.example.com"
    auth:
      type: "private_key_jwt"
      token_path: "/etc/flwd/auth/staging-token.json"
      private_key_path: "/etc/flwd/auth/staging-key.pem"
```

## Next Steps

- [Job Configuration]({{< ref "job-configuration" >}}) - Learn about job-level `config.yaml`
- [Sources & Tree-v1]({{< ref "sources-structure" >}}) - Understand job discovery
- [API Reference]({{< ref "api-reference" >}}) - Programmatic access
