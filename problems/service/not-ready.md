---
title: "not-ready"
description: "Service has started but is not yet ready to serve requests"
problem_type: "service.not_ready"
---

### Scope

This leaf is the canonical `service.not_ready` problem for post-startup readiness failures where the service has started, but readiness checks still fail. It is intentionally narrow: use it only after startup has completed and the service is not yet safe to serve traffic. If startup has not completed yet, use `problems/service/startup-incomplete.md` instead.

### Symptom

Service reports `503 Service Unavailable` with RFC7807 type `https://flowd.org/problems/not-ready`.

### Cause

The service has completed startup but is not ready to serve requests due to:
- Health checks failing
- Required services or dependencies unavailable
- Temporary maintenance mode

### Remediation

- Check the service health endpoint for detailed status
- Verify all required dependencies are available
- Review logs for any errors or warnings
- Wait for the service to become healthy and ready