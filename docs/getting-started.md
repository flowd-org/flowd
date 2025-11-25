---
title: "Getting Started"
weight: 1
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

Welcome to FLOWD. This guide walks you through the first steps: cloning the
repository, building the binary, and running a simple job.

## What is FLOWD?

FLOWD (**Framework for Language-agnostic Orchestration of Workflows Distributed**) is the framework implemented by the `flwd` engine. `flwd` (pronounced "flowed") turns ad-hoc operational scripts into secure, auditable jobs with typed inputs and a unified runtime. Instead of wiring the same scripts differently for laptops, CI and servers, you describe a job once and run it anywhere: locally, in CI, or behind a small HTTP/REST API.

If you just want to see it in action, start with the [Quick Start]({{< ref "quick_start.md" >}}).

## Requirements

To build and run the reference implementation you need:

- A supported OS: Linux, *BSD, macOS or Windows.
- Go toolchain (1.21+ recommended).
- Git.
- Optionally, a rootless container runtime (Podman or Docker) if you want to use
  the container executor.

## Clone and build

Clone the official repository and build the static binary:

```bash
$ git clone https://github.com/flowd-org/flowd.git
$ cd flowd
$ go build ./cmd/flwd
```

This produces a `flwd` binary in the current directory. You can also use any
existing build scripts (`make`, packaging recipes, etc.) as documented in the
repository.

Tip: move `flwd` somewhere on your `PATH` (for example `~/.local/bin`) so it is
available as a command.

```bash
$ mv ./flwd ~/.local/bin/
$ which flwd
/home/you/.local/bin/flwd
```

## Initialise a workspace

flwd discovers jobs from local trees, git sources and add‑ons. A common pattern
is to keep your jobs under a dedicated directory in a repository.

Create a simple workspace and a “hello world” job:

```bash
$ mkdir -p ~/flwd-workspace
$ cd ~/flwd-workspace
$ flwd :init tools/hello-world
```

`:init` scaffolds a minimal job tree at `tools/hello-world/` with a job spec and
a small script implementing the job.

Inspect what was created:

```bash
$ tree tools/hello-world
$ cat tools/hello-world/job.yaml
$ cat tools/hello-world/hello-world.sh
```

## Run your first job

From inside the workspace, list jobs and run the generated example:

```bash
$ flwd :jobs
$ flwd hello-world --name "Your Name"
```

Ask for help and inspect the auto‑generated interface:

```bash
$ flwd hello-world --help
```

The job specification defines arguments, defaults and descriptions; the CLI,
completion and future HTTP/REST API all use the same definition.

## Next steps

Once you have `flwd` built and a workspace initialised, you can:

- follow the [Quick Start]({{< ref "quick_start.md" >}}) for a fast tour of planning and runs;
- enable [Shell Completions]({{< ref "shell-completions.md" >}}) for a better CLI experience;
- learn about [Serve Mode]({{< ref "serve-mode.md" >}}) to expose jobs over HTTP and SSE;
- explore [Sources and Add‑Ons]({{< ref "sources.md" >}}) to load jobs from git and OCI images.

When you are comfortable with the basics, you can start converting your own
scripts into jobs and gradually build a catalogue of reusable operational tools.
