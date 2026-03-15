---
title: "timeout"
description: "Operation or request exceeded allowed deadline"
---

### Symptom

Requests hang or return timeout errors; traces show operations exceeding configured deadlines.

### Cause

- Misconfigured timeouts
- Downstream calls or retries that prolong latency

### Remediation

- Ensure proper timeout propagation and cancellation
- Add timeouts and circuit breakers to downstream calls
- Tune retry/backoff policies to avoid convoy effects
