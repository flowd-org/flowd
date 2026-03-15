---
title: "startup-incomplete"
problem_type: "services.startup_incomplete"
---

### Symptom

Service reports `503 Service Unavailable` with RFC7807 type `https://flowd.org/problems/startup-incomplete`.

### Cause

The service is still initializing and has not completed its startup sequence. This includes:
- Core DB connection not established
- Storage backends not ready
- Required services not yet available

### Remediation

- Wait for the service to complete startup
- Check startup logs for errors or delays
- Verify all required dependencies are available before starting the service