---
title: "Documentation"
---

{{% callout type="warning" %}}
**Disclaimer:**<br>
The flowd project is evolving rapidly and <strong><i>is currently in an active and heavy development phase</i></strong>. As a result, the documentation may not always match the current behavior of the codebase. We are actively working to stabilize the project and aim to reach a stable release as soon as possible.
{{% /callout %}}

# flwd Documentation

`flwd` (pronounced "flowed") is the reference implementation of **FLOWD — Framework for Language-agnostic Orchestration of Workflows Distributed**. It is a small job engine that discovers scripts, enforces typed inputs, and runs them via CLI and HTTP with unified logs.

## Naming & compatibility

This project is **not related to “flowd”, the NetFlow collector by Damien Miller**.

To avoid binary name collisions and respect that existing ecosystem, the runtime and CLI are called **`flwd`** in all commands, config files and packages. The `flowd.org` domain is simply the short, acronym-based URL for FLOWD.

All documentation and packaging keep `flwd` clearly distinct from the existing `flowd` NetFlow daemon, so both can safely coexist on the same machines.


## What you’ll find here

This section collects user-facing guides for installing flwd, running your
first jobs, and understanding the architecture and main components.

## Where to start

- **[Architecture Overview]({{< ref "architecture-overview.md" >}})**  
  High-level view of the engine, components and execution flow.

- **[Quick Start]({{< ref "quick_start.md" >}})**  
  Fast path: list → plan → run a job, enable completion, try the TUI.

- **[Getting Started]({{< ref "getting-started.md" >}})**  
  Clone, build `flwd`, initialise a workspace and run a “hello world” job.

- **[Quickstart (CLI)]({{< ref "quickstart-cli.md" >}})**  
  Slightly deeper CLI tour: planning, runs, JSON output, inspecting history.

## Runtime and usage

- **[Serve Mode (HTTP + SSE)]({{< ref "serve-mode.md" >}})**  
  Run `flwd` as a small HTTP server and call jobs remotely.

- **[Shell Completions]({{< ref "shell-completions.md" >}})**  
  Enable tab-completion for job names, flags and values.

- **[Aliases & Intelligent Completion]({{< ref "aliases-completion.md" >}})**  
  Friendly names for tools and smarter completion hints.

## Sources and packaging

- **[Sources (Local, Git)]({{< ref "sources.md" >}})**  
  Load jobs from local directories and git repositories.

- **[OCI Add-On Sources]({{< ref "oci-addons.md" >}})**  
  Package jobs inside container images and expose them as sources.

- **[Container Executor]({{< ref "container-executor.md" >}})**  
  Run jobs inside rootless containers with secure defaults.

## Diagnostics

- **[Troubleshooting]({{< ref "troubleshooting.md" >}})**  
  Common problems (container runtime, add-ons, sources, auth) and how to fix them.

---

Code repository: https://github.com/flowd-org/flowd

This site currently uses the local docs under `content/docs/`. When we are ready
to import docs from the main code repository, we can mount the `docs/` folder
from `github.com/flowd-org/flowd` into this site via Hugo Modules.

