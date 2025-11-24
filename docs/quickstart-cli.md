---
title: "Quickstart (CLI)"
weight: 4
---

{{% callout type="info" %}}
The documentation may not be up to date. See the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

Run tools from your terminal with live logs and clear results. This page
assumes you already have `flwd` in your `PATH` and at least one job available.

## List, plan and run

List jobs:

```bash
$ flwd :jobs
```

Preview a plan without running anything:

```bash
$ flwd :plan hello-world --name "Alice"
```

Execute the job and stream logs to the console:

```bash
$ flwd hello-world --name "Alice"
```

Ask for structured output (events as JSON):

```bash
$ flwd hello-world --name "Alice" --json
```

What you will see:

- Planning validates inputs and shows which steps will run.
- Runs stream logs and events in real time; secrets are redacted.
- Finished runs are recorded so you can revisit their logs and artifacts later.

Tip: use `--idempotency-key` (or let the CLI generate one) if you are calling
jobs from CI or other automation and want to avoid duplicate effects.

## Inspect past runs

List past runs:

```bash
$ flwd :runs
```

Show details for a specific run:

```bash
$ flwd :runs --id RUN_ID_HERE --json
```

Use `jq` to navigate the run journal:

```bash
$ flwd :runs --json | jq '.runs[0]'
```

## Use the TUI

For a more interactive workflow, the TUI mirrors the CLI but with forms:

```bash
$ flwd :tui
```

From the TUI you can:

- browse jobs and read their descriptions,
- fill in arguments with validation feedback,
- launch runs and watch logs,
- inspect past runs.

The TUI is optional and uses the same engine, so everything you do from there
can also be done via the CLI or HTTP API.

## Remote CLI against a server

When you run `flwd :serve`, the CLI can talk to that server instead of executing
jobs directly on your machine. A minimal example:

```bash
$ flwd :serve --bind 127.0.0.1:8080 --profile secure --dev
```

In another terminal:

```bash
$ export FLOWD_REMOTE=http://127.0.0.1:8080
$ export FLOWD_TOKEN=dev-token
$ flwd :jobs
$ flwd hello-world --name "Alice"
```

The CLI detects the remote configuration and uses the HTTP API, but the commands
stay the same.

For more details, see [Serve Mode (HTTP + SSE)]({{< ref "serve-mode.md" >}}).
