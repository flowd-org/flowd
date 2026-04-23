---
title: Insufficient Scope
type: https://flowd.org/problems/auth/insufficient-scope
status: 403
---

# Insufficient Scope

**Type URI:** `https://flowd.org/problems/auth/insufficient-scope`  
**HTTP Status:** `403 Forbidden`

## Description

The request contains a valid JWT token, but the token's scopes do not include permission for this operation. This is **not** a retryable error.

## Client Handling

Upon receiving this error:

1. Do NOT automatically retry with the same token.
2. Inform the user that they lack the required permissions.
3. If appropriate, prompt the user to obtain a token with elevated scopes or contact an administrator.

## Example `application/problem+json`

```json
{
  "type": "https://flowd.org/problems/auth/insufficient-scope",
  "title": "Insufficient Scope",
  "status": 403,
  "detail": "token missing required scope: job:read"
}
```

## References

* RFC7807 [Problem Details for HTTP APIs](https://datatracker.ietf.org/doc/html/rfc7807)
