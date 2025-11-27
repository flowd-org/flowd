---
title: "Why Not Just Bash or PowerShell?"
weight: 2
---

{{% callout type="info" %}}
This page explains the motivation for flwd as an engine on top of existing
shells. It does not replace the formal reference for job configuration or
security profiles.
{{% /callout %}}

Most teams already have "automation":

- a pile of Bash or PowerShell scripts  
- glued together with SSH, cron and CI jobs  

So a natural question is:

> Why do I need `flwd` at all? Why not just write good Bash/PowerShell?

`flwd` does **not** replace your shell. It treats Bash and PowerShell as
*runtimes* and moves the boring, fragile parts into the engine: arguments,
safety, observability, profiles and tests.

This page walks through where raw scripts hurt, and how flwd changes the
model.

## 1. Unsafe defaults and quoting foot-guns

Bash and PowerShell give you a lot of power with very little ceremony. That is
exactly why small mistakes are easy and expensive.

### Example: spaces in filenames

A classic Bash loop:

```bash
#!/usr/bin/env bash
set -e

for f in $(ls /backup); do
  echo "Processing $f"
  # ...
done
```

Works until there is a file called `db backup.tar.gz`:

- `$f` becomes `db` and then `backup.tar.gz`  
- behaviour depends on current data  
- tests might never exercise the broken case  

The "defensive" version has to remember a lot:

```bash
#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

for f in /backup/*; do
  [ -f "$f" ] || continue
  echo "Processing: $f"
  # ...
done
```

Now consider how many scripts in a typical environment:

- forget `set -Eeuo pipefail`  
- forget to reset `IFS`  
- forget to quote variables  

Each author has to know and remember all of this, all the time.

### How flwd helps

With flwd:

- job configuration is described declaratively (inputs, profiles, sources)  
- the engine starts shells in a **strict, controlled environment**  
- argument binding is driven by a **typed ArgSpec**, not ad-hoc `$1`, `$2`  

You still write the loop in Bash, but:

- strict mode and environment setup live in **one place** (the adapter + engine)  
- the same behaviour is exercised by tests and by real runs  

Individual authors focus on *what the job does*, not on memorising shell
defensive patterns.

## 2. Every script re-implements argument parsing and help

To make a script pleasant to use, you typically need:

- flags and positional arguments  
- `--help` output  
- validation (required, enums, ranges)  
- shell completion (if you are lucky)

In raw Bash that usually looks like a custom parser:

```bash
ENV=""
RETRIES=3

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env)
      ENV="$2"; shift 2;;
    --retries)
      RETRIES="$2"; shift 2;;
    --help|-h)
      usage; exit 0;;
    *)
      echo "Unknown argument: $1" >&2; exit 1;;
  esac
done

[ -n "$ENV" ] || { echo "--env is required" >&2; exit 1; }
```

Problems:

- copy-pasted and tweaked differently in each script  
- validation is inconsistent or missing  
- help text drifts out of sync with actual behaviour  

PowerShell has richer parameter support, but you still need to:

- declare parameters and validation attributes  
- keep docs, scripts and CI in sync by hand

### How flwd changes this

With flwd:

- inputs are described once in **ArgSpec** inside the job configuration  
- the engine generates:
  - consistent CLI flags  
  - `--help` output  
  - shell completion  
  - validation errors before the script starts  

You stop writing custom parsers and usage blocks. You describe:

```yaml
argspec:
  args:
    - name: env
      type: string
      required: true
      enum: [prod, staging, dev]
    - name: retries
      type: int
      default: 3
```

and flwd handles the rest.

## 3. Logging, audit and observability

Ad-hoc scripts typically log via `echo`, sometimes redirected to a file:

- no structured format  
- no correlation ID per run  
- no consistent mapping to metrics or health checks  

It is hard to answer:

- "Who ran this job?"  
- "With which arguments?"  
- "What exactly happened between 03:12:05 and 03:12:10?"  

### Runs as first-class objects

flwd treats each execution as a **run**, not just a process:

- runs have IDs and status  
- events (start, step, error, completion) are recorded  
- logs can be streamed or exported  
- metrics and health endpoints are exposed by the server mode  

This gives you:

- a journal you can query  
- Prometheus-friendly metrics  
- a clear view of what your automation is actually doing  

You do not have to re-invent log prefixes and ad-hoc log files in every
script. The engine provides the envelope.

## 4. No profiles or invariants in raw scripts

Plain scripts run with "whatever they get":

- current user and group  
- ambient capabilities  
- full network access unless constrained externally  
- full filesystem tree unless constrained externally  

There is no standard way to say:

- "this job *must* run rootless with dropped caps and no-new-privileges"  
- "fail closed if we cannot enforce that"  

### flwd profiles and secure execution

flwd introduces **profiles**:

- `secure`  
  - rootless containers  
  - dropped capabilities  
  - `no_new_privileges`  
  - controlled network and filesystem  
  - **fail-closed** if invariants cannot be guaranteed  
- `permissive`  
  - more flexible, still structured  
- `disabled`  
  - full freedom, explicitly chosen  

The engine inspects the container/runtime configuration and refuses to run
under the secure profile if the environment does not match the contract.

Policy and invariants are centralised in the engine, not scattered across
scripts.

## 5. Jobs as software with tests

Most organisations intend to test their scripts. In practice, testing is:

- bespoke CI jobs calling scripts with different arguments  
- no consistent layout for tests and fixtures  
- hard to share or reuse job logic safely  

### flwd jobs are designed to be testable

The flwd model (in the planned 1.0 design) is:

- jobs live alongside their **tests** and configuration  
- `flwd :init` can scaffold a job + tests  
- `flwd :test` can run:
  - on the host  
  - in a dedicated QA container image  

The same adapters, profiles and bindings are used for:

- local development  
- CI  
- production runs  

Instead of re-inventing a test harness per script, you adopt a standard way
to verify jobs.

## 6. What flwd actually does for you

Putting it together:

With raw Bash/PowerShell, each script has to handle:

- argument parsing and `--help`  
- validation and enums  
- environment setup (strict modes, locale, PATH, IFS…)  
- logging and audit trail  
- security profiles and capabilities  
- testing and CI integration  

With flwd, the **engine** provides:

- ArgSpec → flags, `--help`, completion, validation  
- profiles → secure/permissive/disabled, enforced at runtime  
- runs → IDs, journal, events, metrics  
- adapters → consistent behaviour across Bash and PowerShell  
- hooks → `:init`, `:test` and QA images (planned for 1.0)  

You still write small shell scripts. The difference is:

> you focus on *what* the job should do, and let flwd handle the plumbing,
> safety, and observability.

When that trade-off makes sense for you, flwd is the right tool. When it does
not, you can still "just" write a script — but now you have a clear picture
of what you are giving up.
