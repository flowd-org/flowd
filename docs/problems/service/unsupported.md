---
title: "unsupported"
description: "When the service is not supported on this platform or configuration"
---

### Symptom

Requests return 'service unsupported' or related error codes; the service is not expected to run in this environment.

### Cause

The configured service uses platform features or integrations not available in the deployment target.

### Remediation

- Verify platform compatibility matrix in the Core SoT.
- Disable or replace unsupported features, or run the service on a supported platform.
