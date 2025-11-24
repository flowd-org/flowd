<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->

# Security Policy

This document explains how to report security vulnerabilities in **flowd**.

Please **do not** open public GitHub issues for security problems.

---

## Supported versions

Security issues are generally addressed for:

- The current `main` branch
- The most recent tagged releases

Older releases may or may not receive fixes, depending on severity and practicality. If in doubt, report the issue anyway.

---

## Reporting a vulnerability

To report a security vulnerability:

1. Go to the flowd repository on GitHub:
   - `https://github.com/flowd-org/flowd`
2. Use the **“Report a vulnerability”** flow under the **Security** tab  
   or create a **private security advisory**.

If for some reason that’s not possible, you can:

- Open a new issue with **only minimal, non-sensitive details**, and state that you believe you found a security issue, or
- Contact the maintainers through any private channel listed on the project’s GitHub profile.

Public issues must not contain exploit details, credentials, or other sensitive information.

---

## What to include

When reporting a vulnerability, please include:

- A clear description of the issue
- Affected version(s) of flowd (output of `flwd version`)
- Environment details (OS, architecture, container/orchestration setup if relevant)
- Step-by-step instructions to reproduce the issue
- Any relevant logs or output (with secrets removed)

If you have a proof-of-concept, describe it clearly and share it privately.

---

## What to expect

After you report a vulnerability:

1. We will acknowledge receipt of your report as soon as reasonably possible.
2. We will investigate the issue and assess impact and severity.
3. We will work on a fix, and may ask you for more details or validation.
4. Once a fix is ready, we will:
   - Release a patched version (where applicable)
   - Credit reporters if they wish to be credited and it is safe to do so

We do not currently offer a bug bounty program.

---

## Out of scope

The following are generally **not** considered security vulnerabilities in flowd:

- Misconfigurations of third-party services or infrastructure
- Denial-of-service via obviously abusive input or unbounded resource usage in user-provided scripts
- Vulnerabilities in dependencies that cannot be exploited through normal flowd usage
- Issues that require privileged local access beyond a normal user account

If you are unsure whether something is in scope, report it privately and we will confirm.
