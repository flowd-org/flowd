---
title: Problems
type: https://flowd.org/problems
---

# Problems

This section documents canonical problem types used throughout the flwd API.

## Overview

All non-2xx HTTP responses follow RFC7807 and return `application/problem+json`. This directory contains human-readable documentation for each canonical problem type, including:

- Problem type URI
- HTTP status code
- When to expect this error
- How to handle it client-side

## Namespace: auth

* [Invalid Token](auth/invalid-token.md) - 401 Unauthorized due to invalid authentication token
* [Insufficient Scope](auth/insufficient-scope.md) - 403 Forbidden due to missing required scope
