---
title: Explicit Deny
type: https://flowd.org/problems/rbac/explicit-deny
status: 403
---

# Explicit Deny

**Type URI:** `https://flowd.org/problems/rbac/explicit-deny`  
**HTTP Status:** `403 Forbidden`

## Description

The request contains a valid JWT token, but an explicit `!` (deny) rule in the token's scopes matches the request. This is **not** a retryable error.

## When Returned

- A scope-based deny rule (`!`) in the token explicitly denies the requested operation
- The server evaluates all scopes and finds at least one explicit deny that matches the path/resource

## Client Handling

Upon receiving this error:

1. Do NOT automatically retry with the same token.
2. Inform the user that they lack permission for this operation.
3. If appropriate, prompt the user to obtain a token without the deny rule or contact an administrator.

## Example `application/problem+json`

```json
{
  "type": "https://flowd.org/problems/rbac/explicit-deny",
  "title": "Explicit Deny",
  "status": 403,
  "detail": "token contains explicit deny scope: !job:write"
}
```

## References

* RFC7807 [Problem Details for HTTP APIs](https://datatracker.ietf.org/doc/html/rfc7807)
