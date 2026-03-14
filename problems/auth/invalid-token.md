---
title: Invalid Token
type: https://flowd.org/problems/auth/invalid-token
status: 401
---

# Invalid Token

**Type URI:** `https://flowd.org/problems/auth/invalid-token`  
**HTTP Status:** `401 Unauthorized`

## Description

The request contains a JWT token that is invalid. This may be due to:

- Bad signature (token was tampered with or signed with the wrong key)
- Expired `exp` claim
- Future `nbf` claim (clock skew)
- Missing or malformed token

## Client Handling

Upon receiving this error:

1. Discard the current token.
2. Reissue a fresh assertion by calling `/auth/token` with your private_key_jwt credentials.
3. Retry the original request once with the new token.

A second failure MUST be surfaced to the user as an authentication error.

## Example `application/problem+json`

```json
{
  "type": "https://flowd.org/problems/auth/invalid-token",
  "title": "Invalid Token",
  "status": 401,
  "detail": "token signature verification failed: crypto/ed25519: verification error"
}
```

## References

* RFC7807 [Problem Details for HTTP APIs](https://datatracker.ietf.org/doc/html/rfc7807)
