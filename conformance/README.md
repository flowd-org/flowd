# Conformance Harness

A black-box E2E conformance harness for testing the flowd M1 API surface against ULC (Universal Language Client) profiles.

This is a separate Go module (`github.com/flowd-org/flowd/conformance`) that runs against a running `flwd` instance and executes a suite of scenarios defined by ULC profiles.

## Quickstart

1. Build the `flwd` binary:

```bash
cd /path/to/flowd
go build -o ./bin/flwd ./
```

2. Run the conformance harness:

```bash
cd /path/to/flowd/conformance
FLWD_TOKEN=your_api_token go run ./cmd/conformance \
  --flwd-binary ../bin/flwd \
  --report-json ../conformance-report.json
```

The harness automatically selects a free port (18080-18089) for the `flwd` server unless `--bind` is set. It runs all scenarios defined by the default ULC profiles (`ulc.shell.bash,ulc.shell.pwsh`).

### Startup bootstrap prerequisites

Before running conformance, ensure:

- **Binary**: `--flwd-binary` points to the `flwd` binary to test (relative paths are OK; they are resolved to an absolute path).
- **Token**: Provide a valid API token via `--token` or `FLWD_TOKEN`.

The harness creates a temporary run directory, stages the conformance fixture tree into it (`scripts/fixtures/tree-v1`), and starts `flwd` with that directory as its working directory. You do not need to create or symlink a `scripts/` directory in your current working directory.

## Command-line flags

| Flag | Environment | Default | Description |
|------|-------------|---------|-------------|
| `--flwd-binary` | | *(required)* | Path to the `flwd` binary to test (canonicalized to absolute path internally) |
| `--token` | `FLWD_TOKEN` | *(env)* | flowd API token (flag overrides env) |
| `--bind` | | *auto* | Bind address for flwd (empty = auto-select a free port in 18080-18089; set explicitly to override) |
| `--flwd-profile` | | *(none)* | flwd profile to use (`secure|permissive|disabled`) |
| `--ulc-profiles` | | `ulc.shell.bash,ulc.shell.pwsh` | Comma-separated list of ULC profile identifiers (see `DefaultProfiles()` in `conformance/internal/scenarios/registry.go`) |
| `--timeout` | | `5m` | Overall timeout for the conformance run |
| `--scenario-timeout` | | `2m` | Timeout per scenario |
| `--report-json` | | *(none)* | Path to write JSON report (CI example: `--report-json ../conformance-report.json`) |
| `--verbose` | | `false` | Enable verbose logging |

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success — all scenarios passed |
| `1` | Scenario failure — one or more scenarios failed |
| `2` | Usage/config error — missing required flags, invalid configuration, or missing token (see Startup bootstrap failures below) |
| `3` | Infrastructure error — failed to start flwd, process exit during startup, timeout, or other infrastructure issue |

### Startup bootstrap failures

The harness fails fast when bootstrap prerequisites are not met. These errors produce **exit code 2** (usage/config) or **exit code 3** (infrastructure):

| Symptom | Exit code | Cause |
|---------|-----------|-------|
| Missing `--flwd-binary` flag or empty value | `2` | Required configuration missing |
| Token not provided (`FLWD_TOKEN` unset, no `--token`) | `2` | Authentication required |
| Binary path does not exist or is not executable | `3` | Infrastructure: flwd binary unavailable |
| Fixture source tree cannot be found for staging | `3` | Infrastructure: fixture source not found (run from within the repository so `conformance/fixtures/tree-v1` is available) |
| Process exits during startup (pre-/startupz) | `3` | Infrastructure: flwd terminated unexpectedly |
| No free port in 18080-18089 (unless `--bind` set) | `3` | Infrastructure: port range exhausted |

**Example: missing token (exit code 2)**

```bash
FLWD_TOKEN= go run ./cmd/conformance --flwd-binary ../bin/flwd
# Output: Error: missing token (set --token or FLWD_TOKEN)
# Exit: 2
```

## Required check name

When integrating with external systems (e.g., GitHub checks), use the check name **`conformance`**.

## Report JSON

When `--report-json` is specified, the harness writes a JSON report with the following structure:

```json
{
  "suite_meta": {
    "name": "conformance",
    "profiles": ["ulc.shell.bash", "ulc.shell.pwsh"],
    "total_tests": 12
  },
  "scenario_count": 12,
  "passed_count": 10,
  "failed_count": 2,
  "results": [
    {
      "scenario_id": "ulc-smoke",
      "scenario_name": "ULC Smoke Test",
      "profile": "ulc.shell.bash",
      "passed": true,
      "duration_ms": 150,
      "failure": null
    }
  ]
}
```

### Field stability

- **Stable fields** (guaranteed to be present): `suite_meta`, `scenario_count`, `passed_count`, `failed_count`, `results`
- **Stable within `results`**: `scenario_id`, `scenario_name`, `profile`, `passed`, `duration_ms`
- **Best-effort fields** (may be omitted or incomplete on failure): `failure`, `suite_meta.total_tests`

## Security posture

### Token redaction

The harness **never** prints tokens to stdout or stderr:

- The raw token is never logged in error messages
- `Authorization: Bearer` patterns are redacted before output
- JSON reports are defensively scanned for Authorization header patterns

### Report safety

Reports generated with `--report-json` are safe to upload to external systems (e.g., CI artifacts, issue attachments). No secrets are written to the report.

## Debugging

### Finding NDJSON logs

If `flwd` is started by the harness, its NDJSON logs are written to a temporary directory. The exact path is printed at startup when `--verbose` is enabled.

### Rerunning scenarios

Currently, the harness runs all scenarios defined by the selected ULC profiles. Individual scenario reruns are not yet supported (see future work in the main repository).

## Troubleshooting

### CI report artifact

When the conformance harness runs in CI (`.github/workflows/conformance.yml`), the JSON report is uploaded as an artifact named `conformance-report` containing `conformance-report.json`. This artifact is retained for 7 days.

### CI token behavior

The CI workflow distinguishes between trusted paths and fork PRs:

- **Trusted paths** (push to `main`, or PRs on the main repository): The workflow **fails** if `FLWD_TOKEN` is missing, because conformance cannot run without a valid token. Expected log message: `FLWD_TOKEN missing on trusted path (push or internal PR); conformance harness cannot run`

- **Fork PRs**: The workflow **warns** but continues if `FLWD_TOKEN` is missing. Expected log message: `Fork PR without token: conformance harness not executed`. Contributors should run conformance locally before PR submission.

This ensures that maintainers are notified immediately when conformance is skipped on trusted paths, while fork contributors are informed but not blocked by a missing token.

### Reading failure details

In the JSON report, each failed test appears as an item in the `results[]` array with the following fields:

- `scenario_id`: The stable identifier (e.g., `ulc-smoke`)
- `profile`: The ULC profile identifier (e.g., `ulc.shell.bash`, `ulc.shell.pwsh`)
- `failure.actual`: The human-readable failure message explaining why the scenario failed

Example (YAML for readability):

```yaml
results:
  - scenario_id: ulc-smoke
    profile: ulc.shell.bash
    passed: false
    failure:
      actual: "expected status 200, got 500"
```

### Rerunning locally

To rerun the full conformance suite locally with verbose logging and JSON report output:

```bash
cd conformance
FLWD_TOKEN=your_api_token go run ./cmd/conformance \
  --flwd-binary ../bin/flwd \
  --report-json ../conformance-report.json \
  --verbose
```

The harness automatically binds `flwd` to a free port in the 18080-18089 range unless `--bind` is set, and runs all scenarios defined by the selected ULC profiles.

For a specific ULC profile, add `--ulc-profiles`:

```bash
FLWD_TOKEN=your_api_token go run ./cmd/conformance \
  --flwd-binary ../bin/flwd \
  --ulc-profiles ulc.shell.bash \
  --report-json ../conformance-report.json
```

To use a specific bind address, add `--bind`:

```bash
FLWD_TOKEN=your_api_token go run ./cmd/conformance \
  --flwd-binary ../bin/flwd \
  --bind 127.0.0.1:19000 \
  --report-json ../conformance-report.json
```

### Startup bootstrap failures

See `### Startup bootstrap failures` above for the canonical symptom/exit-code table and examples.

### Debugging startup failures

When conformance exits with code 2 or 3, use these steps to diagnose:

1. **Check binary availability**
   ```bash
   ls -la /path/to/flwd && file /path/to/flwd
   ```
   Ensure the path is correct and the binary is executable.

2. **Verify fixture source is present**
   ```bash
   pwd && ls -d conformance/fixtures/tree-v1
   ```
   The harness stages fixtures from the repository into a temp run directory. If you run conformance outside the repo, staging can fail.

3. **Enable verbose logging**
   Add `--verbose` to capture NDJSON logs, which often reveal the root cause before process exit:
   ```bash
   FLWD_TOKEN=your_token go run ./cmd/conformance \
     --flwd-binary ../bin/flwd \
     --verbose
   ```

4. **Test flwd manually**
    Run `flwd` directly to confirm it starts and responds to `/startupz`:
    ```bash
    cd /path/to/flowd && FLWD_TOKEN=test ./bin/flwd :serve --bind 127.0.0.1:18080
    curl -H "Authorization: Bearer $FLWD_TOKEN" http://127.0.0.1:18080/startupz
    ```

## License

AGPL-3.0-or-later — see `LICENSE` in the repository root.
