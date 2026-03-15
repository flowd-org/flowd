# GitHub Actions: common failures and how to diagnose them

This page explains frequent causes of GitHub Actions failures we see in this repository and practical steps to diagnose and fix them. It is intended for maintainers and contributors who encounter failing workflows on PRs or pushes.

## Summary

- Failures are often caused by token/permissions issues, pinned-action breakages, dependency or environment differences, or security scanning tooling (govulncheck / CodeQL) producing signals that need triage.
- Start by checking the workflow run logs and the runner environment (labels, container vs hosted, available secrets).

## Quick checklist

1. Open the Actions run and inspect the failing job and step logs.
2. Look for authentication errors (401/403), missing scopes, or denied API calls. See the auth problems pages for details.
3. Verify whether the run used a personal access token, the default GITHUB_TOKEN, or an action that requires additional permissions.
4. If the failure is in a scheduling or Dependabot-triggered run, check Dependabot config and whether the bot has access.
5. For scanning tools (govulncheck/CodeQL), look at the scanner output and any SARIF/artifact attachments — many failures are benign or require config tuning.

## Common causes and how to diagnose

### 1) Token permissions and scopes

- Symptom: 403 Forbidden or "insufficient scope" messages in logs.
- Cause: Missing repository or organization permissions for the token used by the workflow (GITHUB_TOKEN vs PAT), or a workflow `permissions:` stanza that restricts access.
- Diagnose: Inspect the top-level job logs for the step that makes API calls. Check repository settings → Actions → General → Workflow permissions.
- See also: docs/problems/auth/invalid-token.md and docs/problems/auth/insufficient-scope.md

### 2) Dependabot and pinned actions

- Symptom: sudden failures after Dependabot or when an action version is updated.
- Cause: Upstream action changed behaviour or removed an input; Dependabot PRs may also run with reduced privileges.
- Diagnose: Re-run the workflow with a pinned action SHA or roll back to a known-good action tag; inspect the action repo for breaking changes.

### 3) Govulncheck, CodeQL, and other scanners

- Symptom: job fails due to scanner exit code or missing native tool support.
- Cause: Scanners can return non-zero exit codes when findings are present, or require specific runner environments and caches.
- Diagnose: Inspect the scanner step output and any SARIF artifacts attached to the run; see the security triage guidance for how we treat scanner findings.

### 4) Runner environment and caching

- Symptom: Steps fail with missing binaries, or environment-related errors.
- Cause: Differences between ubuntu-latest vs self-hosted runners, missing caches, or previously cached artifacts with stale state.
- Diagnose: Check the runner labels and the setup steps (actions/checkout + setup-go or setup-python). Reproduce locally with the same runner image if needed.

## Troubleshooting steps (detailed)

1. Re-run the workflow (the "Re-run jobs" button) to see if the failure is transient.
2. Copy the failing step log and search for the first error message; earlier errors are usually the root cause.
3. If the error indicates an authentication problem, check: workflow `permissions:`, repository Actions settings, and whether the run used GITHUB_TOKEN or an explicitly configured PAT.
4. For Dependabot or scheduled runs, verify the bot credentials and whether the run is executing in a fork (forked PRs have restricted secrets).
5. For scanners, export SARIF/artifacts and follow the scanner's triage process; consider adjusting workflow step flags to avoid failing the job on low-confidence findings.

## Cross-links and references

- Problems: auth — docs/problems/auth/invalid-token.md, docs/problems/auth/insufficient-scope.md
- Problems: rbac — docs/problems/rbac/explicit-deny.md, docs/problems/rbac/no-match.md
- API errors and Problem Details: docs/api-reference.md

## Example log snippets

```
Error: GitHub API: 403 Resource not accessible by integration
  -> typically indicates GITHUB_TOKEN missing required permission or the run is from a fork where secrets are not available
```

```
govulncheck: found 2 vulnerabilities
exit status 1
  -> scanner exit causes job failure; open the SARIF artifact to review details
```

## When to open an issue vs PR

- If a workflow needs a permission change or repository setting update, open a repo Issue and reference this docs page.
- If an action or workflow file itself needs changes (pinning, input fixes), create a PR that updates the workflow and include the run id that reproduces the failure.

---

If you need help triaging a specific failing run, include the run URL and the failing job/step in the issue or on the PR.
