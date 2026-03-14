---
title: "Server-Sent Event Problems"
description: "Common SSE failure modes and guidance for resilient streaming endpoints."
---

# Server-Sent Event Problems

This namespace contains canonical SSE problems such as stale cursors, replay mismatches, and connection timeouts with suggested reconciliation patterns.

## stale-cursor

- Description: When a consumer's cursor lags behind the producer and replay yields events that no longer apply.
- Example: replaying an event that references a deleted resource.
- Remediation: provide checkpointing, versioned events, and replay guards.
