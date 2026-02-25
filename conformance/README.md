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
FLWD_TOKEN=your_api_token go run ./cmd/conformance --flwd-binary ../bin/flwd
```

## Command-line flags

| Flag | Environment | Default | Description |
|------|-------------|---------|-------------|
| `--flwd-binary` | | *(required)* | Path to the `flwd` binary to test |
| `--token` | `FLWD_TOKEN` | *(env)* | flowd API token (flag overrides env) |
| `--bind` | | *ignored* | Not currently honored — harness picks a free localhost port via `PickBindAddr()` |
| `--flwd-profile` | | *(none)* | flwd profile to use (e.g., `ulc.shell.bash`) |
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
| `2` | Usage/config error — missing required flags or invalid configuration |
| `3` | Infrastructure error — failed to start flwd, timeout, or other infrastructure issue |

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
      "scenario_id": "ulc_smoke",
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
- `Authorization: Bearer <token>` patterns are redacted before output
- JSON reports are defensively scanned for Authorization header patterns

### Report safety

Reports generated with `--report-json` are safe to upload to external systems (e.g., CI artifacts, issue attachments). No secrets are written to the report.

## Debugging

### Finding NDJSON logs

If `flwd` is started by the harness, its NDJSON logs are written to a temporary directory. The exact path is printed at startup when `--verbose` is enabled.

### Rerunning scenarios

Currently, the harness runs all scenarios defined by the selected ULC profiles. Individual scenario reruns are not yet supported (see future work in the main repository).

## License

MIT — see `LICENSE` in the repository root.
