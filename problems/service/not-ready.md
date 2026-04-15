---
title: Not Ready
type: https://flowd.org/problems/not-ready
status: 503
---

**Type URI:** `https://flowd.org/problems/not-ready`
**HTTP Status:** `503 Service Unavailable`

## Summary

The service has started but is not yet safe to serve traffic.

## When this occurs

- Readiness checks are still failing after startup completes
- Required dependencies are unavailable
- The service is in temporary maintenance mode

## Example `application/problem+json`

```json
{
  "type": "https://flowd.org/problems/not-ready",
  "title": "Not Ready",
  "status": 503,
  "detail": "readiness probe failed: dependency db is unavailable"
}
```

## How to resolve

- Check the service health endpoint for detailed status
- Verify all required dependencies are available
- Review logs for any errors or warnings
- Wait for the service to become healthy and ready

## Scope

This leaf is the canonical `service.not_ready` problem for post-startup
readiness failures. Use it only after startup has completed. If startup has not
completed yet, use `problems/service/startup-incomplete.md` instead.

If the service is already serving but temporarily strained, use
`problems/service/busy.md` instead.

## See also

- `problems/service/startup-incomplete.md` for startup still in progress
