---
title: No Match
type: https://flowd.org/problems/rbac/no-match
status: 404
---

# No Match

**Type URI:** `https://flowd.org/problems/rbac/no-match`  
**HTTP Status:** `404 Not Found`

## Description

No RBAC rule matches the request. This typically indicates that the requested resource or operation is unknown to the authorization system.

## When Returned

- The token scopes do not contain any rule (allow or deny) for the requested path/resource
- The server cannot determine whether the request should be permitted based on available scope rules

## Client Handling

Upon receiving this error:

1. Verify that the resource path and operation are correct.
2. If this is unexpected, contact an administrator to review RBAC configuration.

## Example `application/problem+json`

```json
{
  "type": "https://flowd.org/problems/rbac/no-match",
  "title": "No Match",
  "status": 404,
  "detail": "no RBAC rule matches path=/api/v1/unknown-resource"
}
```

## References

* RFC7807 [Problem Details for HTTP APIs](https://datatracker.ietf.org/doc/html/rfc7807)
