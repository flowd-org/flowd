---
title: "busy"
description: "Service under temporary overload or resource pressure"
---

### Symptom

Requests are rate-limited, queued, or return transient errors indicating overload.

### Cause

- High request rate exceeding configured concurrency limits
- Downstream services causing backpressure

### Remediation

- Introduce throttling with Retry-After headers
- Scale horizontally or raise concurrency limits carefully
- Investigate downstream latencies and add circuit breakers
