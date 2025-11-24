<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->

# Contributing to flowd

Thanks for considering a contribution.

This document explains how to report issues and propose changes in a way that is easy to review and maintain.

---

## 1. Reporting bugs

1. Check existing issues to avoid duplicates.
2. Use the **Bug report** issue template.
3. Include:
   - flowd version (`flwd version`)
   - OS / environment
   - Steps to reproduce
   - Expected vs actual behavior
   - Relevant log/output snippets (without secrets)

Clear, reproducible reports get fixed faster.

---

## 2. Requesting features or improvements

1. Use the **Feature request** issue template.
2. Describe:
   - The problem you want to solve
   - How you’d like flowd to behave
   - Any constraints or non-goals
   - Example CLI/usage if helpful

Feature requests that focus on the problem (not just a specific solution) are easier to reason about.

---

## 3. Working on the code

### 3.1. Setup

- Install Go (version as specified in `go.mod`).
- Clone the repository:
  ```bash
  git clone https://github.com/flowd-org/flowd.git
  cd flowd
  ```
- Make sure tests pass before you start:
  ```bash
  go test ./...
  ```

### 3.2. Branches

Create a feature branch for your work:

```bash
git checkout -b feat/short-description
# or
git checkout -b fix/short-description
# or
git checkout -b docs/short-description
```

Keep branches focused on a small, coherent change.

---

## 4. Submitting a pull request

1. Ensure the code builds and tests pass:
   ```bash
   go test ./...
   ```
2. Update documentation if behavior changes.
3. Open a pull request:
   - Use the PR template.
   - Give a clear title and short summary.
   - Link related issues (e.g. `Fixes #123`).

Reviewers will look for:

- Clear problem statement and motivation
- Minimal, focused changes
- Tests that cover the behavior
- No obvious breaking changes without discussion

---

## 5. Code style

- Follow existing patterns in the codebase.
- Prefer small, composable functions.
- Keep public APIs stable unless there is a strong reason to change them.
- Add tests when fixing bugs or changing behavior.

---

## 6. License

By contributing, you agree that your contributions are licensed under the
same license as the project:

- AGPL-3.0-or-later (see `LICENSE` and `LICENSE-EXCEPTIONS.md`).
