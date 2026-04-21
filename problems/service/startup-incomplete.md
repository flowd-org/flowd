---
title: "Startup Incomplete"
type: "https://flowd.org/problems/startup-incomplete"
status: 503
---

**Type URI:** `https://flowd.org/problems/startup-incomplete`
**HTTP Status:** `503 Service Unavailable`

## Summary

The service is still starting up and is not yet ready to serve traffic.

## When this occurs

- Core DB connection has not been established
- Storage backends are not ready
- Required services are not yet available

## Example `application/problem+json`

```json
{
  "type": "https://flowd.org/problems/startup-incomplete",
  "title": "Startup Incomplete",
  "status": 503,
  "detail": "service initialization still in progress"
}
```

## How to resolve

- Wait for startup to complete
- Check startup logs for errors or delays
- Verify all required dependencies are available before starting the service

## Scope

This leaf is the canonical `service.startup_incomplete` problem while startup is still in progress. If startup has completed but the service still cannot serve requests, use `problems/service/not-ready.md` instead.

Spec scope conformance: this page remains the canonical `service.startup_incomplete` leaf and stays limited to the startup-in-progress case; post-startup readiness failures belong in `problems/service/not-ready.md`.

## See also

- `problems/service/not-ready.md` for post-startup readiness failures