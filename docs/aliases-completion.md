---
title: "Aliases & Intelligent Completion"
weight: 10
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

Give friendly names to tools and get smarter completion hints.

## Aliases

Aliases let you add human-friendly names for jobs or group them under a common
prefix. They are declared in your configuration (for example in `scripts/flwd.yaml`):

```yaml
aliases:
  - from: demo
    to: hello-demo
    description: Shortcut for demo tool

  - from: tools/db/backup
    to: db-backup
    description: Database backup with standard options
```

After reloading, you can use the alias instead of the original name:

```bash
$ flwd hello-demo --name "Avery"
$ flwd db-backup --help
```

Aliases show up in `:jobs` and in the TUI, so everyone sees the same
vocabulary.

## Intelligent completion

The completion engine knows about aliases, arguments and values:

```bash
$ flwd <TAB>
$ flwd hello-<TAB>
$ flwd hello-demo --<TAB>
```

It can also suggest flags and value hints based on the argument specification.

You can inspect the low-level completion endpoint directly to understand what
the shell integration sees:

```bash
$ flwd __complete 1 | jq
$ flwd __complete 4 hello-demo "" | jq
$ flwd __complete 5 hello-demo -- --loud "" | jq
```

These commands emit NDJSON records with suggestions, which the shell-side
scripts translate into actual completions.

As your job catalogue grows, the completion engine stays fast and only returns
what is relevant to the current cursor position.
