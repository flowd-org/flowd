---
title: "startup-incomplete"
description: "Service has not finished initializing and is not yet ready to serve requests"
problem_type: "service.startup_incomplete"
---

### Symptom

Service reports `503 Service Unavailable` with RFC7807 type `https://flowd.org/problems/startup-incomplete`.

### Cause

The service is still initializing and has not completed its startup sequence. This includes:
- Core DB connection not established
- Storage backends not ready
- Required services not yet available

### Scope

This leaf is the canonical `service.startup_incomplete` problem for cases where startup is still in progress. If startup has completed but the service still cannot serve requests, use `problems/service/not-ready.md` instead.

### Remediation

- Wait for the service to complete startup
- Check startup logs for errors or delays
- Verify all required dependencies are available before starting the service
- If startup has completed but the service still cannot serve requests, use `problems/service/not-ready.md` instead