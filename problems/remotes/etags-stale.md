---
title: "ETag stale responses and cache invalidation"
description: "Guidance for handling stale ETag/If-None-Match and cache invalidation when proxying remote resources."
---

# ETags and Stale Responses

When Flowd proxies remote resources, ETag and Last-Modified headers may be used to short-circuit transfer. This page documents common failure modes and recommended patterns.

## Problem

- Remotes sometimes return stale ETag values leading to clients receiving outdated content.

## Guidance

- Verify upstream ETag semantics: strong vs weak validators.
- Prefer conditional requests with If-None-Match and fall back to time-based heuristics when upstream is inconsistent.

## Examples

```http
GET /resource
If-None-Match: "v1"
```

## References

- Core SoT: remotes canonical problem definitions
