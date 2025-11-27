---
title: "Quick Start"
weight: 4
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

Get from zero to useful in minutes. This guide assumes you have already built
`flwd` and have a simple workspace (see [Getting Started]({{< ref "getting-started.md" >}})).

We will:

1. list jobs,
2. plan a run,
3. execute a job and watch logs,
4. enable shell completion.

## 1) List → Plan → Run

From your workspace:

```bash
$ flwd :jobs
```

Plan a job first. Planning validates arguments and shows what would run, without
actually executing it:

```bash
$ flwd :plan hello-world --name "Alice"
```

Now run it:

```bash
$ flwd hello-world --name "Alice"
```

What happens:

- `:jobs` discovers all jobs from local trees, git sources and add‑ons,
- `:plan` validates inputs, profiles and executors and prints a human‑readable
  plan,
- the bare job name executes the plan and streams logs and events to your
  terminal.

## 2) Watch structured output

You can ask for structured JSON output for automation or debugging:

```bash
$ flwd hello-world --name "Alice" --json
```

This prints a stream of events (start, log messages, completion, errors) as
JSON. Tools like `jq` can be used to filter and reshape this.

```bash
$ flwd hello-world --name "Alice" --json | jq '.event, .log // empty'
```

## 3) Explore the TUI

For interactive use, launch the terminal UI:

```bash
$ flwd :tui
```

The TUI lets you browse jobs, fill arguments with forms, run jobs and watch
logs, all from the keyboard. It talks to the same engine and uses the same job
definitions as the CLI.

## 4) Enable basic completion

Turn on shell completion to avoid typing job names and flags by hand. For
example, on Bash:

```bash
$ flwd completion bash > ~/.local/share/flwd.bash
$ echo 'source ~/.local/share/flwd.bash' >> ~/.bashrc
$ source ~/.bashrc
```

Now try typing:

```bash
$ flwd <TAB>
$ flwd hello-world --<TAB>
```

Completions reflect your actual job catalogue and argument specifications.

## Where to go next

- [Quickstart (CLI)]({{< ref "quickstart-cli.md" >}}) – a slightly deeper tour of the CLI.
- [Serve Mode (HTTP + SSE)]({{< ref "serve-mode.md" >}}) – run `flwd` as a small HTTP server.
- [Sources (Local, Git)]({{< ref "sources.md" >}}) – load jobs from other directories and git.
- [OCI Add‑On Sources]({{< ref "oci-addons.md" >}}) – package jobs inside container images.
- [Container Executor]({{< ref "container-executor.md" >}}) – run jobs inside rootless containers.
- [Troubleshooting]({{< ref "troubleshooting.md" >}}) – common errors and how to fix them.
